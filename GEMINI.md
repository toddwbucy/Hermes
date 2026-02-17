# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Vision: Hermes the Psychopomp

**Hermes** is the TUI frontend for **Persephone**, the graph-native task management system inside HADES (semantic graph RAG backed by ArangoDB). The codebase is a port/adaptation of **Sidecar** — an existing Go TUI developer dashboard — being transformed into the visual interface for Persephone's graph-backed workflow.

**The stack**: Persephone (Python, lives in `HADES/core/persephone/`) provides the backend — tasks, sessions, handoffs, workflow state machine, and knowledge graph edges all stored in ArangoDB (`bident` database). Hermes provides the real-time terminal UI on top: kanban boards, session timelines, graph exploration, context assembly visualization.

**Name origin**: Hermes Psychopomp — guide of souls between worlds — bridging the HADES backend to the human developer.

### Persephone Backend (in HADES, `bident` database)

Persephone stores everything as ArangoDB graph nodes + edges:
- **Tasks** (`persephone_tasks`): status workflow (open → in_progress → in_review → closed/blocked), typed as task/bug/epic, with priority, labels, acceptance criteria, parent_key for epic hierarchy
- **Sessions** (`persephone_sessions`): agent fingerprinting (walks process tree to detect claude_code, cursor, codex, etc.), branch tracking, session continuity via `continues` edges
- **Handoffs** (`persephone_handoffs`): structured context transfer (done/remaining/decisions/uncertain + git state capture)
- **Edges** (`persephone_edges`): typed relationships — `implements`, `submitted_review`, `approved`, `blocked_by`, `continues`, `authored_handoff`, `handoff_for`
- **Workflow state machine**: guard-enforced transitions (reviewer != implementer, dependency blocking, block_reason required). All transitions create audit edges.

Query the backend: `HADES_DATABASE=bident hades db aql "FOR doc IN persephone_tasks RETURN doc"`

### Remaining Persephone Phases (Hermes will visualize these)

- **Phase 4**: Handoff system — structured handoff docs as graph nodes
- **Phase 5**: Knowledge graph linking — tasks ↔ papers, files, other tasks, arbitrary docs via edges
- **Phase 6**: Context assembly (THE KILLER FEATURE) — multi-hop graph traversal to pre-assemble all context for a task
- **Phase 7**: CLI integration and agent template updates

## Sidecar Codebase (Being Ported)

The existing Go TUI codebase under `sidecar/`. Module: `github.com/marcus/sidecar`. Built on Bubble Tea (Elm architecture: Model → Update → View).

### Commands

```bash
cd sidecar/

# Build & run
make build                    # → ./bin/sidecar
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

CI runs `go test ./...` and `golangci-lint run ./...` on PRs and main pushes.

### Architecture

#### Plugin System (`internal/plugin/` + `internal/plugins/`)

Plugins implement `plugin.Plugin` (ID, Init, Start, Stop, Update, View, Commands). Registered in `cmd/sidecar/main.go` and managed by `plugin.Registry`.

**Six plugins**: conversations (AI session browser), filebrowser, gitstatus, notes (feature-flagged off), tdmonitor, workspace.

Critical rendering rules:
- **Always constrain `View()` output to the `height` parameter** — plugins must not exceed allocated height or the header scrolls off-screen
- **Never render footers in `View()`** — the app renders a unified footer from `Commands()`. Keep command names to 1 word ("Stage" not "Stage file")
- Failing plugins degrade silently (recorded in `registry.unavailable`, not a crash)

**Epoch mechanism**: `ctx.Epoch` (uint64) increments on project switch. Async messages arriving after a switch are discarded via `plugin.IsStale(ctx, msg)`. Any async message should embed the epoch and implement `EpochMessage`.

#### Adapter System (`internal/adapter/`)

Adapters implement `adapter.Adapter` to provide AI session data (Sessions, Messages, Usage, Watch). Each adapter has a `register.go` with `func init()` that calls `adapter.RegisterFactory(...)`, then is blank-imported in `main.go`.

**To add a new adapter**: implement `Adapter` interface → create `register.go` with init → add blank import to `main.go`.

Optional interfaces extend adapters: `ProjectDiscoverer`, `TargetedRefresher`, `WatchScopeProvider`, `MessageSearcher`.

#### Caching (`internal/adapter/cache/`)

Generic thread-safe LRU cache keyed on file path, invalidated on size/mtime changes. Supports `ByteOffset` for incremental JSONL parsing (only newly-appended data is re-parsed). Performance-critical since sessions can reach 100-500MB.

#### Tiered File Watching (`internal/adapter/tieredwatcher/`)

Two tiers: hot (fsnotify, real-time, limited count) for recently active sessions, cold (polling) for everything else. Limits OS file descriptor usage.

#### Inter-Plugin Communication

All plugins receive all `tea.Msg` via broadcast. Use `app.FocusPlugin(id)` to switch focus. `filebrowser.NavigateToFileMsg{Path}` to open files. Pattern: `tea.Batch(app.FocusPlugin("file-browser"), func() tea.Msg { return filebrowser.NavigateToFileMsg{Path: path} })`.

#### Config & State

- **Config** (`~/.config/sidecar/config.json`): User settings (plugins, keymap, themes, projects)
- **State** (`~/.config/sidecar/state.json`): Runtime preferences (diff mode, pane widths, active plugin per project)

#### Feature Flags (`internal/features/`)

Three-level priority: CLI override > config file > default. Currently 3 flags: `tmux_interactive_input` (on), `tmux_inline_edit` (on), `notes_plugin` (off).

#### Theme System

Built-in themes (`internal/styles/themes.go`) + 453 community themes (`internal/community/`). Per-project theme support in config. Live preview during selection.

### Key Patterns

- **Version injection**: Set via `-ldflags "-X main.Version=..."` at build time; falls back to git revision
- **Panic recovery**: Plugin lifecycle methods (Init/Start/Stop) are wrapped in `defer recover()` in the registry
- **CGO**: `mattn/go-sqlite3` requires CGO; release builds use `CGO_ENABLED=0` with `modernc.org/sqlite` as pure-Go fallback
- **Worktree awareness**: State tracks last active worktree per main repo; `app.GetMainWorktreePath()` resolves worktree → main repo mapping

### td Integration (Being Replaced by Persephone)

Sidecar currently integrates with `td` (task management tool, `github.com/marcus/td`). The tdmonitor plugin watches for task changes. **Hermes replaces this with Persephone's ArangoDB-backed graph** — tasks become graph nodes with typed edges instead of flat SQLite records.

## HADES CLI

Use the `/hades` skill for HADES queries. Key commands for Persephone development:

```bash
# Query persephone data
HADES_DATABASE=bident hades db aql "FOR doc IN persephone_tasks FILTER doc.status == 'open' RETURN doc"
HADES_DATABASE=bident hades db aql "FOR doc IN persephone_sessions FILTER doc.ended_at == null RETURN doc"
HADES_DATABASE=bident hades db aql "FOR e IN persephone_edges RETURN e"

# Collections in bident
HADES_DATABASE=bident hades db collections
HADES_DATABASE=bident hades db stats --all
```
