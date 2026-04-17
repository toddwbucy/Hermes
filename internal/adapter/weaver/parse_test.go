package weaver

import (
	"path/filepath"
	"strings"
	"testing"
)

const fixturePath = "testdata/trace-fixture.jsonl"

func TestReadSpansFixture(t *testing.T) {
	spans, err := readSpans(fixturePath)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	if len(spans) != 6 {
		t.Fatalf("want 6 spans, got %d", len(spans))
	}

	// Resource fields use OTel/OpenInference dotted keys, not snake_case.
	if got := spans[0].Resource.RunID; got != "run_01HZTEST" {
		t.Errorf("Resource.RunID: got %q, want run_01HZTEST", got)
	}
	if got := spans[0].Resource.ServiceName; got != "weaver-herobench" {
		t.Errorf("Resource.ServiceName: got %q, want weaver-herobench", got)
	}
	if got := spans[0].Resource.OpenInferenceVersion; got != "1.0" {
		t.Errorf("Resource.OpenInferenceVersion: got %q, want 1.0", got)
	}
	// Flattened extras should be preserved for display.
	if !strings.Contains(string(spans[0].Resource.AdditionalAttributes), "host.name") {
		t.Errorf("AdditionalAttributes missing host.name: %s", spans[0].Resource.AdditionalAttributes)
	}
}

func TestSpanKindAndAttrs(t *testing.T) {
	spans, err := readSpans(fixturePath)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}

	cases := map[string]string{
		"0000000000000001": "AGENT",
		"0000000000000002": "CHAIN",
		"0000000000000003": "LLM",
		"0000000000000004": "TOOL",
		"0000000000000005": "LLM",
		"0000000000000006": "TOOL",
	}
	for i := range spans {
		want, ok := cases[spans[i].SpanID]
		if !ok {
			continue
		}
		if got := spans[i].SpanKind(); got != want {
			t.Errorf("span %s kind: got %q, want %q", spans[i].SpanID, got, want)
		}
	}

	// LLM token attributes round-trip as numbers.
	for i := range spans {
		if spans[i].SpanID == "0000000000000003" {
			if got := spans[i].AttrUint64("llm.token_count.prompt"); got != 1200 {
				t.Errorf("LLM #1 prompt tokens: got %d, want 1200", got)
			}
			if got := spans[i].AttrString("llm.model_name"); got != "qwen3-coder-30b" {
				t.Errorf("LLM #1 model: got %q, want qwen3-coder-30b", got)
			}
		}
	}
}

func TestSessionIDFromSpans(t *testing.T) {
	spans, err := readSpans(fixturePath)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	if got := sessionIDFromSpans(spans, fixturePath); got != "run_01HZTEST" {
		t.Errorf("sessionIDFromSpans: got %q, want run_01HZTEST", got)
	}

	// Fallback: when no spans carry a run_id, derive from filename stem.
	got := sessionIDFromSpans(nil, "/tmp/logs/trace-abc-def.jsonl")
	if got != "abc-def" {
		t.Errorf("filename fallback: got %q, want abc-def", got)
	}
}

func TestBuildMessagesAttachesToolToLLMParent(t *testing.T) {
	spans, err := readSpans(fixturePath)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	msgs := buildMessages(spans)

	// 2 LLM spans + 1 orphan TOOL (parent is CHAIN, not LLM) = 3 messages.
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}

	// First message should be LLM #1 with the HeroBenchObserve tool attached.
	llm1 := msgs[0]
	if llm1.Role != "assistant" {
		t.Errorf("first message role: got %q, want assistant", llm1.Role)
	}
	if llm1.Model != "qwen3-coder-30b" {
		t.Errorf("first message model: got %q, want qwen3-coder-30b", llm1.Model)
	}
	if llm1.InputTokens != 1200 || llm1.OutputTokens != 340 {
		t.Errorf("first message tokens: got %d/%d, want 1200/340", llm1.InputTokens, llm1.OutputTokens)
	}
	if len(llm1.ToolUses) != 1 {
		t.Fatalf("first message: want 1 ToolUse, got %d", len(llm1.ToolUses))
	}
	if llm1.ToolUses[0].Name != "HeroBenchObserve" {
		t.Errorf("ToolUse name: got %q, want HeroBenchObserve", llm1.ToolUses[0].Name)
	}
	if llm1.ToolUses[0].ID != "call_abc" {
		t.Errorf("ToolUse ID: got %q, want call_abc", llm1.ToolUses[0].ID)
	}
	if !strings.Contains(llm1.ToolUses[0].Input, "task_keywords") {
		t.Errorf("ToolUse input missing task_keywords: %q", llm1.ToolUses[0].Input)
	}
	if !strings.Contains(llm1.ToolUses[0].Output, "position") {
		t.Errorf("ToolUse output missing position: %q", llm1.ToolUses[0].Output)
	}

	// Second message should be LLM #2 with no tools.
	llm2 := msgs[1]
	if len(llm2.ToolUses) != 0 {
		t.Errorf("LLM #2 should have no tools, got %d", len(llm2.ToolUses))
	}

	// Third message should be the orphan HeroBenchAct (no LLM ancestor),
	// carrying isError=true from the ERROR status.
	orphan := msgs[2]
	if len(orphan.ToolUses) != 1 || orphan.ToolUses[0].Name != "HeroBenchAct" {
		t.Fatalf("orphan tool message wrong shape: %+v", orphan.ToolUses)
	}
	if len(orphan.ContentBlocks) != 1 || !orphan.ContentBlocks[0].IsError {
		t.Errorf("orphan tool ContentBlocks should have IsError=true: %+v", orphan.ContentBlocks)
	}
}

func TestBuildMessagesSortsByTimestamp(t *testing.T) {
	spans, err := readSpans(fixturePath)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	msgs := buildMessages(spans)
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Timestamp.Before(msgs[i-1].Timestamp) {
			t.Errorf("messages out of order: msg[%d].Timestamp %v < msg[%d].Timestamp %v",
				i, msgs[i].Timestamp, i-1, msgs[i-1].Timestamp)
		}
	}
}

func TestAggregateTokensAndCounts(t *testing.T) {
	spans, err := readSpans(fixturePath)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	in, out := aggregateTokens(spans)
	if in != 2700 {
		t.Errorf("input tokens: got %d, want 2700", in)
	}
	if out != 420 {
		t.Errorf("output tokens: got %d, want 420", out)
	}
	if got := countKind(spans, "LLM"); got != 2 {
		t.Errorf("LLM count: got %d, want 2", got)
	}
	if got := countKind(spans, "TOOL"); got != 2 {
		t.Errorf("TOOL count: got %d, want 2", got)
	}
}

func TestReadSpansSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace-mixed.jsonl")
	mustWrite(t, path, strings.Join([]string{
		`{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"good","startTimeUnixNano":1,"endTimeUnixNano":2,"attributes":{},"status":{"code":"OK"},"resource":{"service.name":"x","weaver.run_id":"r","openinference.spec_version":"1.0"},"scope":{"name":"weaver-trace","version":"0.1.0"}}`,
		`{not valid json`,
		``,
		`{"traceId":"00000000000000000000000000000002","spanId":"0000000000000002","name":"also good","startTimeUnixNano":3,"endTimeUnixNano":4,"attributes":{},"status":{"code":"OK"},"resource":{"service.name":"x","weaver.run_id":"r","openinference.spec_version":"1.0"},"scope":{"name":"weaver-trace","version":"0.1.0"}}`,
	}, "\n"))

	spans, err := readSpans(path)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("want 2 spans (malformed line skipped), got %d", len(spans))
	}
}
