package weaver

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/toddwbucy/hermes/internal/adapter"
)

// mustWrite is shared by tests in this package — writes content to path or
// fails the test.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// projectWithFixture stages the fixture trace file in a temporary
// `<projectRoot>/logs/` so adapter methods can find it.
func projectWithFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	src, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(logs, "trace-run_01HZTEST.jsonl")
	if err := os.WriteFile(dst, src, 0644); err != nil {
		t.Fatalf("write fixture copy: %v", err)
	}
	return root
}

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.ID() != "weaver" {
		t.Errorf("ID: got %q, want weaver", a.ID())
	}
	if a.Name() != "Weaver" {
		t.Errorf("Name: got %q, want Weaver", a.Name())
	}
	if a.Icon() == "" {
		t.Errorf("Icon should be non-empty")
	}
	caps := a.Capabilities()
	for _, c := range []adapter.Capability{adapter.CapSessions, adapter.CapMessages, adapter.CapUsage} {
		if !caps[c] {
			t.Errorf("capability %s should be true", c)
		}
	}
	if caps[adapter.CapWatch] {
		t.Errorf("CapWatch is deferred and should be false")
	}
}

func TestDetect(t *testing.T) {
	a := New()

	// No logs directory → false, no error.
	empty := t.TempDir()
	ok, err := a.Detect(empty)
	if err != nil {
		t.Fatalf("Detect on empty: %v", err)
	}
	if ok {
		t.Errorf("Detect should be false when no trace files present")
	}

	// Empty projectRoot is a no-op (don't scan filesystem root).
	ok, err = a.Detect("")
	if err != nil {
		t.Fatalf("Detect empty root: %v", err)
	}
	if ok {
		t.Errorf("Detect should be false for empty projectRoot")
	}

	// With a fixture present → true.
	root := projectWithFixture(t)
	ok, err = a.Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !ok {
		t.Errorf("Detect should be true when trace files exist")
	}
}

func TestSessionsAndMessagesRoundtrip(t *testing.T) {
	a := New()
	root := projectWithFixture(t)

	sessions, err := a.Sessions(root)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != "run_01HZTEST" {
		t.Errorf("Session ID: got %q, want run_01HZTEST (from resource.weaver.run_id)", s.ID)
	}
	if s.AdapterID != "weaver" {
		t.Errorf("AdapterID: got %q, want weaver", s.AdapterID)
	}
	if s.MessageCount != 2 {
		t.Errorf("MessageCount: got %d, want 2 (LLM spans only)", s.MessageCount)
	}
	if s.TotalTokens != 2700+420 {
		t.Errorf("TotalTokens: got %d, want %d", s.TotalTokens, 2700+420)
	}
	if s.FileSize <= 0 {
		t.Errorf("FileSize should be > 0, got %d", s.FileSize)
	}

	// Messages must be retrievable by the session ID returned above —
	// this covers the sessionIndex caching path.
	msgs, err := a.Messages(s.ID)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("want 3 messages (2 LLM + 1 orphan tool), got %d", len(msgs))
	}

	// Unknown session → nil, no error (so the UI can refresh gracefully).
	msgs, err = a.Messages("nonexistent")
	if err != nil {
		t.Fatalf("Messages(unknown): %v", err)
	}
	if msgs != nil {
		t.Errorf("Messages(unknown) should return nil, got %d", len(msgs))
	}
}

func TestUsage(t *testing.T) {
	a := New()
	root := projectWithFixture(t)

	// Must call Sessions first so the adapter caches the session→path map.
	if _, err := a.Sessions(root); err != nil {
		t.Fatalf("Sessions: %v", err)
	}

	stats, err := a.Usage("run_01HZTEST")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if stats.TotalInputTokens != 2700 {
		t.Errorf("TotalInputTokens: got %d, want 2700", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 420 {
		t.Errorf("TotalOutputTokens: got %d, want 420", stats.TotalOutputTokens)
	}
	if stats.MessageCount != 2 {
		t.Errorf("MessageCount: got %d, want 2", stats.MessageCount)
	}
}

func TestSessionsKeepsPartialTraces(t *testing.T) {
	// A trace file that mixes valid spans with a malformed line should
	// still surface as a session — readSpans tolerates bad lines, and
	// Sessions must not drop the file just because scanner.Err is non-nil
	// or because some lines were skipped.
	a := New()
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(logs, "trace-partial.jsonl")
	mustWrite(t, path, `{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"good","startTimeUnixNano":1700000000000000000,"endTimeUnixNano":1700000001000000000,"attributes":{"openinference.span.kind":"AGENT"},"status":{"code":"OK"},"resource":{"service.name":"x","weaver.run_id":"run_partial","openinference.spec_version":"1.0"},"scope":{"name":"weaver-trace","version":"0.1.0"}}
{not valid json
`)

	sessions, err := a.Sessions(root)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("partial trace should still surface a session, got %d", len(sessions))
	}
	if sessions[0].ID != "run_partial" {
		t.Errorf("session ID: got %q, want run_partial", sessions[0].ID)
	}
}

func TestSessionsEmptyTraceUsesFileMtimeNotNow(t *testing.T) {
	// A trace file with no parseable spans must not show a recent
	// CreatedAt/UpdatedAt — that would make broken sessions look freshly
	// active. The file's mtime is the right anchor.
	a := New()
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(logs, "trace-empty.jsonl")
	mustWrite(t, path, "")

	// Backdate the file so we can prove the session inherits its mtime.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	before := time.Now()
	sessions, err := a.Sessions(root)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("empty trace should still surface as a session, got %d", len(sessions))
	}

	s := sessions[0]
	// UpdatedAt should sit near the file's mtime, well before "now".
	if s.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt should not be after Sessions() was called: %v vs %v", s.UpdatedAt, before)
	}
	// 1 hour of slop on either side of the backdated mtime.
	if s.UpdatedAt.Before(old.Add(-time.Hour)) || s.UpdatedAt.After(old.Add(time.Hour)) {
		t.Errorf("UpdatedAt should be near the file mtime (%v), got %v", old, s.UpdatedAt)
	}
	// Empty session must NOT be marked active — that would mislead the UI
	// into thinking a broken/abandoned trace is live.
	if s.IsActive {
		t.Errorf("empty session should not be IsActive")
	}
}

func TestSessionsDisambiguatesDuplicateRunIDs(t *testing.T) {
	// Two trace files with the same run_id (e.g., user copied a fixture
	// or HEROBENCH_RUN_ID was reused) must each surface as their own
	// session, not silently shadow each other in sessionIndex.
	a := New()
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logs, "trace-first.jsonl"), src, 0644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logs, "trace-second.jsonl"), src, 0644); err != nil {
		t.Fatalf("write second: %v", err)
	}

	sessions, err := a.Sessions(root)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions (no shadowing), got %d", len(sessions))
	}

	ids := map[string]bool{}
	for _, s := range sessions {
		if ids[s.ID] {
			t.Errorf("duplicate session ID %q — disambiguation failed", s.ID)
		}
		ids[s.ID] = true
	}

	// Each unique ID must resolve to its own file when Messages is called.
	for _, s := range sessions {
		msgs, err := a.Messages(s.ID)
		if err != nil {
			t.Fatalf("Messages(%s): %v", s.ID, err)
		}
		if len(msgs) == 0 {
			t.Errorf("Messages(%s) returned empty — likely path collision", s.ID)
		}
	}
}

func TestMessagesAndUsagePreservePartialTraces(t *testing.T) {
	// If readSpans returns parsed spans plus a non-fatal error, Messages
	// and Usage must still surface the parsed content. A malformed line
	// alone won't trigger scanner.Err (readSpans tolerates it), but the
	// guard `len(spans) == 0` is what matters: a partial result must not
	// be discarded just because err != nil. We exercise the success path
	// against a partial trace as the most realistic check.
	a := New()
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(logs, "trace-partial-msg.jsonl")
	mustWrite(t, path, `{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"chain","startTimeUnixNano":1700000000000000000,"endTimeUnixNano":1700000001000000000,"attributes":{"openinference.span.kind":"CHAIN"},"status":{"code":"OK"},"resource":{"service.name":"x","weaver.run_id":"run_partial_msg","openinference.spec_version":"1.0"},"scope":{"name":"weaver-trace","version":"0.1.0"}}
{"traceId":"00000000000000000000000000000001","spanId":"0000000000000002","parentSpanId":"0000000000000001","name":"llm","startTimeUnixNano":1700000002000000000,"endTimeUnixNano":1700000003000000000,"attributes":{"openinference.span.kind":"LLM","llm.model_name":"qwen3","llm.token_count.prompt":100,"llm.token_count.completion":50},"status":{"code":"OK"},"resource":{"service.name":"x","weaver.run_id":"run_partial_msg","openinference.spec_version":"1.0"},"scope":{"name":"weaver-trace","version":"0.1.0"}}
{this is broken json
`)

	if _, err := a.Sessions(root); err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	msgs, err := a.Messages("run_partial_msg")
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("want 1 LLM message from partial trace, got %d", len(msgs))
	}
	stats, err := a.Usage("run_partial_msg")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if stats.TotalInputTokens != 100 || stats.TotalOutputTokens != 50 {
		t.Errorf("partial-trace usage: got %d/%d, want 100/50",
			stats.TotalInputTokens, stats.TotalOutputTokens)
	}
}

func TestWatchReturnsClosedChannel(t *testing.T) {
	a := New()
	ch, closer, err := a.Watch(t.TempDir())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if closer == nil {
		t.Fatal("Watch closer should be non-nil")
	}
	defer func() { _ = closer.Close() }()

	// Channel should be closed (no events) — Watch is deferred.
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("Watch channel should be closed, got an event")
		}
	default:
		t.Errorf("Watch channel should be closed (recv immediately), got blocked")
	}

	// Closing should not error.
	if err := closer.Close(); err != nil {
		t.Errorf("closer.Close: %v", err)
	}

	// Quiet io.EOF reference for compilers that flag the import as unused
	// when the file shrinks.
	_ = io.EOF
}
