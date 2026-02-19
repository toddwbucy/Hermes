# Hermes

**Hermes** is a real-time terminal UI (TUI) for [Persephone](https://github.com/toddwbucy/hades), the graph-native task management backend inside HADES. It gives you a keyboard-driven dashboard for AI agent sessions, kanban task boards, git status, and workspace context — all in the terminal.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm architecture), Apache 2.0 licensed (derived from [Sidecar](https://github.com/mvorwaller/sidecar) by Marcus Vorwaller).

---

## The Stack

```
┌─────────────────────────────────┐
│  Hermes  (this repo)            │  Terminal UI — kanban, sessions, git, workspace
├─────────────────────────────────┤
│  Persephone  (HADES backend)    │  Task & session graph — Python, ArangoDB
├─────────────────────────────────┤
│  HADES / ArangoDB  (bident DB)  │  Graph storage — tasks, sessions, handoffs, edges
└─────────────────────────────────┘
```

Persephone stores everything as ArangoDB graph nodes + edges: tasks with a guard-enforced workflow state machine, AI agent session fingerprints, structured handoffs, and knowledge graph relationships. Hermes surfaces all of it in real time.

---

## Features

### Persephone Task Board
- Kanban columns: **Open → In Progress → In Review → Blocked → Closed**
- Mouse + keyboard navigation; per-column scroll
- Task detail view with notes, status transitions, and dependency graph
- Live polling from ArangoDB (`bident` database)

### AI Session Viewer (Conversations)
- Tracks Claude Code, Gemini, Pi, and other AI agent sessions
- Split-pane: session list + conversation transcript
- Tiered file watching (fsnotify hot tier + polling cold tier) for low fd usage
- Session classification, branch tracking, worktree-aware

### Git Status
- Staged/unstaged diffs with syntax highlighting (Chroma)
- File browser integration — open any changed file directly

### File Browser
- Navigate project files, preview with syntax highlighting
- Integrates with git status and other plugins via inter-plugin messaging

### Workspace
- tmux pane capture for terminal context
- Project-scoped view switching

### Theming
- 453 community themes + built-in themes
- Override any color token in `~/.config/hermes/config.json`

---

## Installation

**Requirements**: Go 1.25+, ArangoDB with HADES/Persephone configured (for the task board)

```bash
# Install from source (recommended — picks up git version info)
git clone https://github.com/toddwbucy/hermes
cd hermes
make install-dev

# Or plain install
go install github.com/toddwbucy/hermes/cmd/hermes@latest
```

Run it:

```bash
hermes
```

Config is auto-created at `~/.config/hermes/config.json` on first run.

---

## Configuration

`~/.config/hermes/config.json`:

```json
{
  "projects": {
    "mode": "single",
    "root": "."
  },
  "plugins": {
    "git-status": {
      "enabled": true,
      "refreshInterval": "1s"
    },
    "conversations": {
      "enabled": true,
      "claudeDataDir": "~/.claude"
    },
    "td-monitor": {
      "enabled": true,
      "dbPath": ".todos/issues.db"
    }
  },
  "ui": {
    "showClock": true,
    "theme": { "name": "default" }
  }
}
```

Runtime preferences (active plugin, scroll positions, etc.) are stored in `~/.config/hermes/state.json`.

---

## Key Bindings

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle plugins |
| `?` | Toggle help / key hints |
| `q` / `Ctrl+C` | Quit |
| `↑` / `↓` / `j` / `k` | Navigate lists |
| `Enter` | Select / open detail |
| `Esc` | Back / close modal |

Plugin-specific bindings are shown in the footer bar while that plugin is focused.

---

## Architecture

### Plugin System

All views are plugins implementing `plugin.Plugin`:

```go
type Plugin interface {
    ID() string
    Init(ctx *Context) tea.Cmd
    Update(msg tea.Msg) tea.Cmd
    View(width, height int) string
    Commands() []Command
    // ...
}
```

Registered in `cmd/hermes/main.go`, managed by `plugin.Registry`. Failing plugins degrade silently. An **epoch mechanism** (`ctx.Epoch`) discards stale async messages after project switches.

**Current plugins**: `conversations`, `filebrowser`, `gitstatus`, `persephone`, `workspace`, `tdmonitor`

### Adapter System

Adapters (`internal/adapter/`) parse AI session data from different agents. Self-registering via `func init()` — adding a new agent means implementing the interface and adding a blank import.

### Inter-Plugin Messaging

All plugins receive all `tea.Msg` via broadcast. Cross-plugin actions use typed messages:

```go
// Navigate git-status → file browser
tea.Batch(
    app.FocusPlugin("file-browser"),
    func() tea.Msg { return filebrowser.NavigateToFileMsg{Path: path} },
)
```

---

## Development

```bash
make build          # → ./bin/hermes
make install-dev    # Install to GOBIN with git version tag
make test           # go test ./...
make test-v         # Verbose tests
make fmt            # go fmt ./...
make lint           # Lint new changes vs main branch
make lint-all       # Lint full codebase
```

> **Note**: `make build` outputs to `./bin/hermes`. To update the binary on your PATH, use `make install-dev` (installs to `~/go/bin/hermes`).

### Adding a Plugin

1. Create `internal/plugins/<name>/plugin.go` implementing `plugin.Plugin`
2. Register in `cmd/hermes/main.go`
3. Critical: always constrain `View()` to the `height` parameter — overflow pushes the header off-screen
4. Never render a footer in `View()` — the app renders a unified footer from `Commands()`

### Adding an Adapter

1. Implement `adapter.Adapter` in `internal/adapter/<name>/`
2. Add `register.go` with `func init() { adapter.Register(...) }`
3. Blank-import in `cmd/hermes/main.go`

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

Derived from [Sidecar](https://github.com/mvorwaller/sidecar) by Marcus Vorwaller (MIT). Original license and copyright preserved in [NOTICE](NOTICE).
