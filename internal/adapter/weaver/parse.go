package weaver

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"time"

	"github.com/toddwbucy/hermes/internal/adapter"
)

// readSpans reads every JSONL line of a trace file into Span values. Malformed
// lines are skipped rather than aborting the parse — a partial trace is more
// useful than no trace.
func readSpans(path string) ([]Span, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	var spans []Span
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s Span
		if err := json.Unmarshal(line, &s); err != nil {
			continue
		}
		spans = append(spans, s)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return spans, err
	}
	return spans, nil
}

// unixNanoToTime converts a u64 nanosecond timestamp to time.Time in UTC.
func unixNanoToTime(n uint64) time.Time {
	return time.Unix(0, int64(n)).UTC()
}

// buildMessages reshapes a flat span list into a Hermes message stream.
//
// Design:
//   - Each LLM span becomes an assistant Message. Prompt/completion token
//     counts come from `llm.token_count.*` attributes.
//   - Each TOOL span becomes a ToolUse on the nearest enclosing LLM message,
//     with `tool.parameters` as the input and the span's `output.value`
//     attribute as the output. If a TOOL span has no LLM ancestor, it is
//     rendered as a standalone tool-only assistant message so it's still
//     visible in the UI.
//   - Messages are sorted by start time so the conversation reads in
//     chronological order.
func buildMessages(spans []Span) []adapter.Message {
	// Index spans by span_id for parent lookup.
	byID := make(map[string]*Span, len(spans))
	for i := range spans {
		byID[spans[i].SpanID] = &spans[i]
	}

	// For each TOOL span, find the enclosing LLM span (if any).
	toolsByLLM := make(map[string][]*Span)
	var orphanTools []*Span
	for i := range spans {
		s := &spans[i]
		if s.SpanKind() != "TOOL" {
			continue
		}
		llm := findAncestorKind(s, byID, "LLM")
		if llm == nil {
			orphanTools = append(orphanTools, s)
			continue
		}
		toolsByLLM[llm.SpanID] = append(toolsByLLM[llm.SpanID], s)
	}

	var messages []adapter.Message
	for i := range spans {
		s := &spans[i]
		if s.SpanKind() != "LLM" {
			continue
		}
		messages = append(messages, llmSpanToMessage(s, toolsByLLM[s.SpanID]))
	}
	for _, t := range orphanTools {
		messages = append(messages, toolOnlyMessage(t))
	}

	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})
	return messages
}

func findAncestorKind(s *Span, byID map[string]*Span, kind string) *Span {
	current := s.ParentSpanID
	// Bounded walk guards against pathological parent cycles.
	for range 128 {
		if current == "" {
			return nil
		}
		parent, ok := byID[current]
		if !ok {
			return nil
		}
		if parent.SpanKind() == kind {
			return parent
		}
		current = parent.ParentSpanID
	}
	return nil
}

func llmSpanToMessage(llm *Span, tools []*Span) adapter.Message {
	model := llm.AttrString("llm.model_name")
	prompt := llm.AttrUint64("llm.token_count.prompt")
	completion := llm.AttrUint64("llm.token_count.completion")

	var blocks []adapter.ContentBlock
	var toolUses []adapter.ToolUse
	for _, t := range tools {
		id := t.AttrString("tool.call_id")
		name := t.AttrString("tool.name")
		input := t.AttrString("tool.parameters")
		if input == "" {
			// tool.parameters may have been serialized as structured JSON;
			// fall back to the raw attribute bytes.
			if raw, ok := t.Attributes["tool.parameters"]; ok {
				input = string(raw)
			}
		}
		output := t.AttrString("output.value")
		isErr := t.Status.Code == "ERROR"

		toolUses = append(toolUses, adapter.ToolUse{
			ID:     id,
			Name:   name,
			Input:  input,
			Output: output,
		})
		blocks = append(blocks, adapter.ContentBlock{
			Type:       "tool_use",
			ToolUseID:  id,
			ToolName:   name,
			ToolInput:  input,
			ToolOutput: output,
			IsError:    isErr,
		})
	}

	return adapter.Message{
		ID:        llm.SpanID,
		Role:      "assistant",
		Content:   "",
		Timestamp: unixNanoToTime(llm.StartTimeUnixNano),
		Model:     model,
		TokenUsage: adapter.TokenUsage{
			InputTokens:  int(prompt),
			OutputTokens: int(completion),
		},
		ToolUses:      toolUses,
		ContentBlocks: blocks,
	}
}

func toolOnlyMessage(t *Span) adapter.Message {
	input := t.AttrString("tool.parameters")
	if input == "" {
		if raw, ok := t.Attributes["tool.parameters"]; ok {
			input = string(raw)
		}
	}
	output := t.AttrString("output.value")
	isErr := t.Status.Code == "ERROR"
	return adapter.Message{
		ID:        t.SpanID,
		Role:      "assistant",
		Timestamp: unixNanoToTime(t.StartTimeUnixNano),
		ToolUses: []adapter.ToolUse{{
			ID:     t.AttrString("tool.call_id"),
			Name:   t.AttrString("tool.name"),
			Input:  input,
			Output: output,
		}},
		ContentBlocks: []adapter.ContentBlock{{
			Type:       "tool_use",
			ToolUseID:  t.AttrString("tool.call_id"),
			ToolName:   t.AttrString("tool.name"),
			ToolInput:  input,
			ToolOutput: output,
			IsError:    isErr,
		}},
	}
}
