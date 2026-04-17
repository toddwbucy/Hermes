package weaver

import (
	"io"
	"os"
	"path/filepath"
	"testing"

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
