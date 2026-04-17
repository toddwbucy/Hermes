// Package weaver implements a Hermes adapter for WeaverTools OpenInference
// trace files. Each `<projectRoot>/logs/trace-*.jsonl` file is surfaced as
// one session, with its LLM and TOOL spans rendered as assistant messages
// + tool calls in the UI.
package weaver

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/toddwbucy/hermes/internal/adapter"
)

const (
	adapterID   = "weaver"
	adapterName = "Weaver"
	adapterIcon = "\u25A3" // ▣ — mnemonic for "spans in a trace"
	traceGlob   = "trace-*.jsonl"
	logsDirName = "logs"
)

// Adapter implements adapter.Adapter for Weaver trace files.
//
// Stateless by design for the first pass — Detect/Sessions/Messages each
// rescan the logs directory. Trace files are small (one line per span,
// order of tens of KB per benchmark run) so a cache is not yet needed.
type Adapter struct {
	// sessionIndex maps run_id -> trace file path. Populated by Sessions()
	// and reused by Messages() to resolve the file without reading every
	// trace line again.
	mu           sync.RWMutex
	sessionIndex map[string]string
}

// New constructs a Weaver adapter.
func New() *Adapter {
	return &Adapter{sessionIndex: make(map[string]string)}
}

func (a *Adapter) ID() string   { return adapterID }
func (a *Adapter) Name() string { return adapterName }
func (a *Adapter) Icon() string { return adapterIcon }

// Detect returns true when at least one trace file exists under the
// project's `logs/` directory.
func (a *Adapter) Detect(projectRoot string) (bool, error) {
	paths, err := a.traceFiles(projectRoot)
	if err != nil {
		return false, err
	}
	return len(paths) > 0, nil
}

// Capabilities advertises read-only access — Watch is deferred.
func (a *Adapter) Capabilities() adapter.CapabilitySet {
	return adapter.CapabilitySet{
		adapter.CapSessions: true,
		adapter.CapMessages: true,
		adapter.CapUsage:    true,
		adapter.CapWatch:    false,
	}
}

// Sessions returns one Session per trace file under `<projectRoot>/logs/`.
// The session's ID is the trace's run_id (read from the first span's
// resource.run_id); if no spans are parseable, the filename stem is used
// as a fallback so the session still appears in the list.
func (a *Adapter) Sessions(projectRoot string) ([]adapter.Session, error) {
	paths, err := a.traceFiles(projectRoot)
	if err != nil {
		return nil, err
	}

	newIndex := make(map[string]string, len(paths))
	sessions := make([]adapter.Session, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		spans, err := readSpans(p)
		// A scanner error after some good lines still returns those lines —
		// keep the partial trace rather than dropping the whole file. Only
		// skip when nothing parsed.
		if err != nil && len(spans) == 0 {
			continue
		}

		runID := sessionIDFromSpans(spans, p)
		// Disambiguate duplicate run_ids (e.g., same trace file copied,
		// or HEROBENCH_RUN_ID reused across runs) by appending the file
		// stem. Without this, the second file silently shadows the first
		// in sessionIndex and Messages/Usage resolve the wrong path.
		if prev, exists := newIndex[runID]; exists && prev != p {
			runID = runID + "::" + strings.TrimSuffix(filepath.Base(p), ".jsonl")
		}
		newIndex[runID] = p

		// File mtime is the right fallback for spanless or malformed traces.
		// Using time.Now() would make broken sessions look freshly active and
		// sort to the top of the list.
		first, last := spanTimeRange(spans, info.ModTime().UTC())
		inputTok, outputTok := aggregateTokens(spans)
		msgCount := countKind(spans, "LLM")

		sessions = append(sessions, adapter.Session{
			ID:              runID,
			Name:            runID,
			AdapterID:       adapterID,
			AdapterName:     adapterName,
			AdapterIcon:     adapterIcon,
			CreatedAt:       first,
			UpdatedAt:       last,
			Duration:        last.Sub(first),
			IsActive:        time.Since(last) < 5*time.Minute,
			TotalTokens:     inputTok + outputTok,
			MessageCount:    msgCount,
			FileSize:        info.Size(),
			Path:            p,
			SessionCategory: adapter.SessionCategoryInteractive,
			CWD:             projectRoot,
		})
	}

	a.mu.Lock()
	a.sessionIndex = newIndex
	a.mu.Unlock()

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Messages returns the message stream reconstructed from the trace's LLM
// and TOOL spans. An unknown session ID returns nil without error so the
// UI can refresh rather than surfacing a failure.
func (a *Adapter) Messages(sessionID string) ([]adapter.Message, error) {
	path := a.sessionPath(sessionID)
	if path == "" {
		return nil, nil
	}
	spans, err := readSpans(path)
	// Mirror Sessions(): keep partial traces. A scanner error after some
	// good lines still yields usable spans — surface them rather than
	// failing the whole call.
	if err != nil && len(spans) == 0 {
		return nil, err
	}
	return buildMessages(spans), nil
}

// Usage aggregates token counts across every LLM span in the session.
func (a *Adapter) Usage(sessionID string) (*adapter.UsageStats, error) {
	path := a.sessionPath(sessionID)
	if path == "" {
		return &adapter.UsageStats{}, nil
	}
	spans, err := readSpans(path)
	if err != nil && len(spans) == 0 {
		return nil, err
	}
	inputTok, outputTok := aggregateTokens(spans)
	return &adapter.UsageStats{
		TotalInputTokens:  inputTok,
		TotalOutputTokens: outputTok,
		MessageCount:      countKind(spans, "LLM"),
	}, nil
}

// Watch is not yet implemented — returns a closed channel. The fsnotify
// wiring is straightforward but out of scope for the initial adapter.
func (a *Adapter) Watch(projectRoot string) (<-chan adapter.Event, io.Closer, error) {
	ch := make(chan adapter.Event)
	close(ch)
	return ch, nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// traceFiles lists all trace-*.jsonl files under `<projectRoot>/logs/`.
func (a *Adapter) traceFiles(projectRoot string) ([]string, error) {
	if projectRoot == "" {
		return nil, nil
	}
	dir := filepath.Join(projectRoot, logsDirName)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, traceGlob))
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func (a *Adapter) sessionPath(sessionID string) string {
	a.mu.RLock()
	p, ok := a.sessionIndex[sessionID]
	a.mu.RUnlock()
	if ok {
		return p
	}
	return ""
}

func sessionIDFromSpans(spans []Span, path string) string {
	for i := range spans {
		if spans[i].Resource.RunID != "" {
			return spans[i].Resource.RunID
		}
	}
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".jsonl")
	return strings.TrimPrefix(base, "trace-")
}

func spanTimeRange(spans []Span, fallback time.Time) (time.Time, time.Time) {
	if len(spans) == 0 {
		return fallback, fallback
	}
	var first, last uint64
	first = spans[0].StartTimeUnixNano
	last = spans[0].EndTimeUnixNano
	for i := range spans {
		if spans[i].StartTimeUnixNano < first {
			first = spans[i].StartTimeUnixNano
		}
		if spans[i].EndTimeUnixNano > last {
			last = spans[i].EndTimeUnixNano
		}
	}
	return unixNanoToTime(first), unixNanoToTime(last)
}

func aggregateTokens(spans []Span) (int, int) {
	var in, out int
	for i := range spans {
		if spans[i].SpanKind() != "LLM" {
			continue
		}
		in += int(spans[i].AttrUint64("llm.token_count.prompt"))
		out += int(spans[i].AttrUint64("llm.token_count.completion"))
	}
	return in, out
}

func countKind(spans []Span, kind string) int {
	var n int
	for i := range spans {
		if spans[i].SpanKind() == kind {
			n++
		}
	}
	return n
}
