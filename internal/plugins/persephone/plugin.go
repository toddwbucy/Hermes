package persephone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toddwbucy/hermes/internal/arango"
	"github.com/toddwbucy/hermes/internal/mouse"
	appmsg "github.com/toddwbucy/hermes/internal/msg"
	persephoneData "github.com/toddwbucy/hermes/internal/persephone"
	"github.com/toddwbucy/hermes/internal/plugin"
)

const (
	pluginID   = "persephone"
	pluginName = "tasks"
	pluginIcon = "P"

	pollInterval = 2 * time.Second
)

// viewState tracks which sub-view is active.
type viewState int

const (
	viewBoard viewState = iota
	viewDetail
	viewStatusModal
	viewNotesModal
	viewSetup
	viewNotConnected
)

// Plugin provides a Persephone task board as a hermes plugin.
type Plugin struct {
	ctx     *plugin.Context
	focused bool

	// Data layer
	store    *persephoneData.Store
	database string

	// View state
	view      viewState
	board     *boardModel
	detail    *detailModel
	setup     *setupModel
	statusMdl *statusModal
	notesMdl  *notesModal

	// Mouse support
	mouseHandler *mouse.Handler

	// Connection state
	connected    bool
	connectError string

	// Dimensions
	width  int
	height int
}

// New creates a new Persephone plugin.
func New() *Plugin {
	return &Plugin{}
}

// ID returns the plugin identifier.
func (p *Plugin) ID() string { return pluginID }

// Name returns the plugin display name.
func (p *Plugin) Name() string { return pluginName }

// Icon returns the plugin icon character.
func (p *Plugin) Icon() string { return pluginIcon }

// Init initializes the plugin with context.
func (p *Plugin) Init(ctx *plugin.Context) error {
	p.ctx = ctx
	p.board = newBoardModel()
	p.detail = newDetailModel()
	p.mouseHandler = mouse.NewHandler()
	p.setup = nil
	p.connected = false

	// Resolve database name: env > .hermes/config.yaml > setup wizard
	p.database = resolveDatabase(ctx.WorkDir)

	if p.database == "" {
		p.view = viewSetup
		p.setup = newSetupModel(ctx.WorkDir)
		return nil
	}

	// Create arango client and store
	client, err := arango.NewClient(p.database)
	if err != nil {
		p.view = viewNotConnected
		p.connectError = err.Error()
		return nil
	}

	p.store = persephoneData.NewStore(client)

	// Test connection
	if err := p.store.Ping(); err != nil {
		p.view = viewNotConnected
		p.connectError = err.Error()
		return nil
	}

	p.connected = true
	p.view = viewBoard
	return nil
}

// Start begins the plugin's async operations.
func (p *Plugin) Start() tea.Cmd {
	if !p.connected {
		return nil
	}
	return tea.Batch(p.fetchTasks(), p.schedulePoll())
}

// Stop cleans up resources.
func (p *Plugin) Stop() {}

// Update handles messages.
func (p *Plugin) Update(msg tea.Msg) (plugin.Plugin, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)

	case tea.MouseMsg:
		return p.handleMouse(msg)

	case tasksFetchedMsg:
		if msg.err != nil {
			p.ctx.Logger.Warn("persephone: fetch failed", "error", msg.err)
			return p, p.schedulePoll()
		}
		p.board.updateTasks(msg.tasks)
		return p, p.schedulePoll()

	case taskDetailMsg:
		if msg.err != nil {
			p.ctx.Logger.Warn("persephone: detail fetch failed", "error", msg.err)
			return p, nil
		}
		p.detail.update(msg.task, msg.sessions, msg.handoff, msg.edges)
		return p, nil

	case pollTickMsg:
		if !p.connected {
			return p, nil
		}
		return p, p.fetchTasks()

	case SetupCompleteMsg:
		p.database = msg.Database
		// Reinitialize with new database
		if err := p.Init(p.ctx); err != nil {
			return p, nil
		}
		return p, p.Start()

	case taskNoteAddedMsg:
		if plugin.IsStale(p.ctx, msg) {
			return p, nil
		}
		p.view = viewDetail
		p.notesMdl = nil
		if msg.err != nil {
			p.ctx.Logger.Warn("persephone: note add failed", "error", msg.err)
			return p, appmsg.ShowToast("Error: "+msg.err.Error(), 3*time.Second)
		}
		return p, tea.Batch(
			p.fetchTasks(),
			p.fetchTaskDetail(msg.taskKey),
			appmsg.ShowToast("Note saved", 2*time.Second),
		)

	case taskStatusChangedMsg:
		p.view = viewDetail
		p.statusMdl = nil
		if msg.err != nil {
			p.ctx.Logger.Warn("persephone: status change failed", "error", msg.err)
			return p, appmsg.ShowToast("Error: "+msg.err.Error(), 3*time.Second)
		}
		return p, tea.Batch(
			p.fetchTasks(),
			p.fetchTaskDetail(msg.taskKey),
			appmsg.ShowToast(fmt.Sprintf("Status → %s", msg.newStatus), 2*time.Second),
		)

	case plugin.PluginFocusedMsg:
		if p.connected {
			return p, p.fetchTasks()
		}
		return p, nil
	}

	return p, nil
}

// handleKey routes key events to the active sub-view.
func (p *Plugin) handleKey(msg tea.KeyMsg) (plugin.Plugin, tea.Cmd) {
	switch p.view {
	case viewBoard:
		switch msg.String() {
		case "j", "down":
			p.board.moveDown()
		case "k", "up":
			p.board.moveUp()
		case "h", "left":
			p.board.moveLeft()
		case "l", "right":
			p.board.moveRight()
		case "r":
			return p, p.fetchTasks()
		case "enter":
			if task := p.board.selectedTask(); task != nil {
				p.view = viewDetail
				p.detail.setTask(task)
				return p, p.fetchTaskDetail(task.Key)
			}
		}

	case viewDetail:
		switch msg.String() {
		case "esc", "q":
			p.view = viewBoard
		case "j", "down":
			p.detail.scrollDown()
		case "k", "up":
			p.detail.scrollUp()
		case "s":
			if t := p.detail.task; t != nil {
				sm := newStatusModal(t.Key, t.Status)
				if sm != nil {
					p.statusMdl = sm
					p.view = viewStatusModal
				}
			}
		case "n":
			if t := p.detail.task; t != nil {
				p.notesMdl = newNotesModal(t.Key)
				p.view = viewNotesModal
			}
		}

	case viewStatusModal:
		if p.statusMdl != nil {
			action, cmd := p.statusMdl.handleKey(msg)
			switch action {
			case "change":
				newStatus := p.statusMdl.selectedStatus()
				blockReason := p.statusMdl.blockReason()
				return p, p.transitionTask(p.statusMdl.taskKey, newStatus, blockReason)
			case "cancel":
				p.view = viewDetail
				p.statusMdl = nil
			}
			return p, cmd
		}

	case viewNotesModal:
		if p.notesMdl != nil {
			action, cmd := p.notesMdl.handleKey(msg)
			switch action {
			case "save":
				content := p.notesMdl.noteContent()
				if content == "" {
					return p, appmsg.ShowToast("Note is empty", 2*time.Second)
				}
				note := persephoneData.TaskNote{
					Content:   content,
					Author:    "hermes-ui",
					CreatedAt: time.Now().UTC(),
				}
				return p, p.appendNote(p.notesMdl.taskKey, note)
			case "cancel":
				p.view = viewDetail
				p.notesMdl = nil
			}
			return p, cmd
		}

	case viewSetup:
		if p.setup != nil {
			return p.setup.handleKey(p, msg)
		}
	}

	return p, nil
}

// Hit region IDs for mouse support.
const (
	regionBoard    = "board"
	regionTaskCard = "task-card"
	regionDetail   = "detail"
)

// handleMouse routes mouse events to the active sub-view.
func (p *Plugin) handleMouse(msg tea.MouseMsg) (plugin.Plugin, tea.Cmd) {
	switch p.view {
	case viewBoard:
		action := p.mouseHandler.HandleMouse(msg)
		switch action.Type {
		case mouse.ActionClick:
			if action.Region != nil && action.Region.ID == regionTaskCard {
				if idx, ok := action.Region.Data.(int); ok {
					task := p.board.selectByIndex(idx)
					if task != nil {
						p.view = viewDetail
						p.detail.setTask(task)
						return p, p.fetchTaskDetail(task.Key)
					}
				}
			}
		case mouse.ActionScrollUp:
			p.board.moveUp()
		case mouse.ActionScrollDown:
			p.board.moveDown()
		}

	case viewDetail:
		action := p.mouseHandler.HandleMouse(msg)
		switch action.Type {
		case mouse.ActionScrollUp:
			p.detail.scrollUp()
		case mouse.ActionScrollDown:
			p.detail.scrollDown()
		}

	case viewStatusModal:
		if p.statusMdl != nil && p.statusMdl.m != nil {
			action := p.statusMdl.m.HandleMouse(msg, p.statusMdl.mouseHandler)
			switch action {
			case "change":
				newStatus := p.statusMdl.selectedStatus()
				blockReason := p.statusMdl.blockReason()
				return p, p.transitionTask(p.statusMdl.taskKey, newStatus, blockReason)
			case "cancel":
				p.view = viewDetail
				p.statusMdl = nil
			}
		}

	case viewNotesModal:
		if p.notesMdl != nil && p.notesMdl.m != nil {
			action := p.notesMdl.m.HandleMouse(msg, p.notesMdl.mouseHandler)
			switch action {
			case "save":
				content := p.notesMdl.noteContent()
				if content == "" {
					return p, appmsg.ShowToast("Note is empty", 2*time.Second)
				}
				note := persephoneData.TaskNote{
					Content:   content,
					Author:    "hermes-ui",
					CreatedAt: time.Now().UTC(),
				}
				return p, p.appendNote(p.notesMdl.taskKey, note)
			case "cancel":
				p.view = viewDetail
				p.notesMdl = nil
			}
		}
	}

	return p, nil
}

// View renders the plugin.
func (p *Plugin) View(width, height int) string {
	p.width = width
	p.height = height

	switch p.view {
	case viewBoard:
		return p.board.view(width, height, p.mouseHandler)
	case viewDetail:
		return p.detail.view(width, height)
	case viewStatusModal:
		bg := p.detail.view(width, height)
		if p.statusMdl != nil {
			return p.statusMdl.render(bg, width, height)
		}
		return bg
	case viewNotesModal:
		bg := p.detail.view(width, height)
		if p.notesMdl != nil {
			return p.notesMdl.render(bg, width, height)
		}
		return bg
	case viewSetup:
		if p.setup != nil {
			return p.setup.view(width, height)
		}
	case viewNotConnected:
		return renderNotConnected(width, height, p.database, p.connectError)
	}

	return ""
}

// IsFocused returns whether this plugin has focus.
func (p *Plugin) IsFocused() bool { return p.focused }

// SetFocused sets the focus state.
func (p *Plugin) SetFocused(f bool) { p.focused = f }

// Commands returns the plugin's available commands.
func (p *Plugin) Commands() []plugin.Command {
	if !p.focused {
		return nil
	}

	switch p.view {
	case viewBoard:
		return []plugin.Command{
			{ID: "nav", Name: "Navigate", Description: "Move cursor", Context: pluginID, Priority: 1},
			{ID: "open", Name: "Open", Description: "View task detail", Context: pluginID, Priority: 2},
			{ID: "refresh", Name: "Refresh", Description: "Refresh tasks", Context: pluginID, Priority: 3},
		}
	case viewDetail:
		return []plugin.Command{
			{ID: "back", Name: "Back", Description: "Return to board", Context: pluginID, Priority: 1},
			{ID: "nav", Name: "Scroll", Description: "Scroll detail", Context: pluginID, Priority: 2},
			{ID: "status", Name: "Status", Description: "Change status", Context: pluginID, Priority: 3},
			{ID: "note", Name: "Note", Description: "Add note", Context: pluginID, Priority: 4},
		}
	case viewNotesModal:
		return []plugin.Command{
			{ID: "save", Name: "Save", Description: "Save note (ctrl+s)", Context: pluginID, Priority: 1},
			{ID: "back", Name: "Cancel", Description: "Close modal", Context: pluginID, Priority: 2},
		}
	case viewStatusModal:
		return []plugin.Command{
			{ID: "select", Name: "Select", Description: "Choose status", Context: pluginID, Priority: 1},
			{ID: "back", Name: "Cancel", Description: "Close modal", Context: pluginID, Priority: 2},
		}
	}

	return nil
}

// FocusContext returns the current focus context for keybindings.
func (p *Plugin) FocusContext() string { return pluginID }

// ConsumesTextInput implements plugin.TextInputConsumer.
// Returns true when the setup wizard or status modal block-reason input is active.
func (p *Plugin) ConsumesTextInput() bool {
	if p.view == viewSetup && p.setup != nil {
		return true
	}
	if p.view == viewStatusModal && p.statusMdl != nil {
		return p.statusMdl.consumesTextInput()
	}
	if p.view == viewNotesModal && p.notesMdl != nil {
		return p.notesMdl.consumesTextInput()
	}
	return false
}

// Diagnostics returns health/status info for the diagnostics panel.
func (p *Plugin) Diagnostics() []plugin.Diagnostic {
	if !p.connected {
		status := "disconnected"
		detail := p.connectError
		if p.database == "" {
			status = "unconfigured"
			detail = "no database configured"
		}
		return []plugin.Diagnostic{{
			ID:     pluginID,
			Status: status,
			Detail: detail,
		}}
	}

	total := p.board.totalTasks()
	return []plugin.Diagnostic{{
		ID:     pluginID,
		Status: "ok",
		Detail: fmt.Sprintf("%d tasks", total),
	}}
}

// --- Messages ---

type tasksFetchedMsg struct {
	tasks []persephoneData.Task
	err   error
}

type taskDetailMsg struct {
	task     *persephoneData.Task
	sessions []persephoneData.Session
	handoff  *persephoneData.Handoff
	edges    []persephoneData.Edge
	err      error
}

type pollTickMsg struct{}

type taskStatusChangedMsg struct {
	taskKey   string
	newStatus string
	err       error
}

type taskNoteAddedMsg struct {
	taskKey string
	epoch   uint64
	err     error
}

func (m taskNoteAddedMsg) GetEpoch() uint64 { return m.epoch }

// SetupCompleteMsg is sent when the setup wizard completes.
type SetupCompleteMsg struct {
	Database string
}

// --- Commands ---

func (p *Plugin) fetchTasks() tea.Cmd {
	store := p.store
	return func() tea.Msg {
		tasks, err := store.ListTasks()
		return tasksFetchedMsg{tasks: tasks, err: err}
	}
}

func (p *Plugin) fetchTaskDetail(key string) tea.Cmd {
	store := p.store
	return func() tea.Msg {
		task, err := store.GetTask(key)
		if err != nil {
			return taskDetailMsg{err: err}
		}
		sessions, _ := store.TaskSessions(key)
		handoff, _ := store.LatestHandoff(key)
		edges, _ := store.TaskEdges(key)
		return taskDetailMsg{task: task, sessions: sessions, handoff: handoff, edges: edges}
	}
}

func (p *Plugin) appendNote(taskKey string, note persephoneData.TaskNote) tea.Cmd {
	store := p.store
	epoch := p.ctx.Epoch
	return func() tea.Msg {
		err := store.AppendNote(taskKey, note)
		return taskNoteAddedMsg{taskKey: taskKey, epoch: epoch, err: err}
	}
}

func (p *Plugin) transitionTask(taskKey, newStatus, blockReason string) tea.Cmd {
	store := p.store
	return func() tea.Msg {
		err := store.TransitionTask(taskKey, newStatus, blockReason)
		return taskStatusChangedMsg{taskKey: taskKey, newStatus: newStatus, err: err}
	}
}

func (p *Plugin) schedulePoll() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return pollTickMsg{}
	})
}

// --- Config Resolution ---

// resolveDatabase determines which HADES database to use.
// Priority: HADES_DATABASE env > .hermes/config.yaml > empty (triggers setup).
func resolveDatabase(workDir string) string {
	// 1. Environment variable (highest priority, same as HADES CLI)
	if db := os.Getenv("HADES_DATABASE"); db != "" {
		return db
	}

	// 2. Per-workspace config
	configPath := filepath.Join(workDir, ".hermes", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err == nil {
		// Simple YAML parsing for "database: xxx" — avoid full YAML dep
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "database:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "database:"))
				val = strings.Trim(val, `"'`)
				if val != "" {
					return val
				}
			}
		}
	}

	// 3. No config found — will trigger setup wizard
	return ""
}
