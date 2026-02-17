# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project: Hermes the Psychopomp

**Hermes** is the TUI frontend for **Persephone**, the graph-native task management system inside HADES (semantic graph RAG backed by ArangoDB). Built on Bubble Tea (Elm architecture: Model → Update → View).

**Module**: `github.com/toddwbucy/hermes`
**License**: Apache 2.0 (derived from Sidecar by Marcus Vorwaller, MIT — see NOTICE)
**Config dir**: `~/.config/hermes/`

**The stack**: Persephone (Python, in `HADES/core/persephone/`) provides the backend — tasks, sessions, handoffs, workflow state machine, knowledge graph edges in ArangoDB (`bident` database). Hermes provides the real-time terminal UI: kanban boards, session timelines, graph exploration, context assembly visualization.

### Persephone Backend (`bident` database)

Persephone stores everything as ArangoDB graph nodes + edges:
- **Tasks** (`persephone_tasks`): workflow (open → in_progress → in_review → closed/blocked), typed as task/bug/epic
- **Sessions** (`persephone_sessions`): agent fingerprinting via process tree, branch tracking, `continues` edges
- **Handoffs** (`persephone_handoffs`): structured context transfer (done/remaining/decisions/uncertain + git state)
- **Edges** (`persephone_edges`): `implements`, `submitted_review`, `approved`, `blocked_by`, `continues`, `authored_handoff`, `handoff_for`
- **Workflow state machine**: guard-enforced transitions, audit edges on every state change

Remaining phases: 4 (handoffs), 5 (knowledge graph linking), 6 (context assembly — THE KILLER FEATURE), 7 (CLI integration)

## Commands

```bash
# Build & run
make build                    # → ./bin/hermes
make install-dev              # Install with git version info

# Tests
make test                     # go test ./...
make test-v                   # Verbose
go test ./internal/adapter/claudecode/...   # Single package
go test -run TestCacheGet ./internal/adapter/cache/...  # Single test

# Lint & format
make fmt                      # go fmt ./...
make lint                     # New issues only (vs main branch)
make lint-all                 # Full codebase
make fmt-check-all            # Check all formatting
```

## Architecture

### Plugin System (`internal/plugin/` + `internal/plugins/`)

Plugins implement `plugin.Plugin` (ID, Init, Start, Stop, Update, View, Commands). Registered in `cmd/hermes/main.go`, managed by `plugin.Registry`.

**Current plugins**: conversations, filebrowser, gitstatus, notes (feature-flagged off), tdmonitor (to be replaced by Persephone plugin), workspace.

Critical rendering rules:
- **Always constrain `View()` to the `height` parameter** — overflow causes header to scroll off-screen
- **Never render footers in `View()`** — the app renders a unified footer from `Commands()`. Keep names to 1 word
- Failing plugins degrade silently (recorded in `registry.unavailable`)

**Epoch mechanism**: `ctx.Epoch` increments on project switch. Async messages after a switch are discarded via `plugin.IsStale(ctx, msg)`.

### Adapter System (`internal/adapter/`)

Adapters implement `adapter.Adapter` for AI session data. Self-registering via `register.go` with `func init()` → blank-imported in `main.go`.

**To add a new adapter**: implement interface → create `register.go` with init → add blank import to `main.go`.

### Caching (`internal/adapter/cache/`)

Generic LRU cache, keyed on file path, invalidated on size/mtime. Supports `ByteOffset` for incremental JSONL parsing.

### Tiered File Watching (`internal/adapter/tieredwatcher/`)

Hot tier (fsnotify, real-time, limited count) + cold tier (polling). Limits file descriptor usage.

### Inter-Plugin Communication

All plugins receive all `tea.Msg` via broadcast. `app.FocusPlugin(id)` switches focus. Pattern: `tea.Batch(app.FocusPlugin("file-browser"), func() tea.Msg { return filebrowser.NavigateToFileMsg{Path: path} })`.

### Config & State

- **Config** (`~/.config/hermes/config.json`): User settings
- **State** (`~/.config/hermes/state.json`): Runtime preferences

### Feature Flags, Themes, Key Patterns

- Feature flags: CLI override > config > default. 3 flags currently.
- 453 community themes (`internal/community/`) + built-in themes
- Version injection via `-ldflags "-X main.Version=..."`, panic recovery in plugin lifecycle, CGO for sqlite3 (pure-Go fallback in releases)

## HADES CLI

```bash
HADES_DATABASE=bident hades db aql "FOR doc IN persephone_tasks FILTER doc.status == 'open' RETURN doc"
HADES_DATABASE=bident hades db aql "FOR doc IN persephone_sessions FILTER doc.ended_at == null RETURN doc"
HADES_DATABASE=bident hades db collections
HADES_DATABASE=bident hades db stats --all
```
