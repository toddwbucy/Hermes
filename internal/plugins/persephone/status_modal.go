package persephone

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/toddwbucy/hermes/internal/modal"
	"github.com/toddwbucy/hermes/internal/mouse"
	persephoneData "github.com/toddwbucy/hermes/internal/persephone"
	"github.com/toddwbucy/hermes/internal/ui"
)

// statusModal manages the status-change picker for a task.
type statusModal struct {
	taskKey       string
	currentStatus string

	// Modal state
	m            *modal.Modal
	selectedIdx  int
	transitions  []string // valid target statuses
	mouseHandler *mouse.Handler

	// Block reason input (shown when "blocked" is selected)
	blockInput      textinput.Model
	needsBlockInput bool

	// Dimensions
	width int
}

// newStatusModal creates a status modal for the given task.
func newStatusModal(taskKey, currentStatus string) *statusModal {
	transitions := persephoneData.ValidTransitions[currentStatus]
	if len(transitions) == 0 {
		return nil
	}

	sm := &statusModal{
		taskKey:       taskKey,
		currentStatus: currentStatus,
		transitions:   transitions,
		mouseHandler:  mouse.NewHandler(),
	}

	sm.blockInput = textinput.New()
	sm.blockInput.Placeholder = "Why is this blocked?"
	sm.blockInput.Width = 40

	return sm
}

// selectedStatus returns the currently selected target status.
func (sm *statusModal) selectedStatus() string {
	if sm.selectedIdx < 0 || sm.selectedIdx >= len(sm.transitions) {
		return ""
	}
	return sm.transitions[sm.selectedIdx]
}

// buildModal constructs the modal with the current state.
func (sm *statusModal) buildModal(screenWidth int) {
	modalW := ui.ModalWidthMedium
	if modalW > screenWidth-4 {
		modalW = screenWidth - 4
	}
	if modalW < 40 {
		modalW = 40
	}

	if sm.m != nil && sm.width == modalW {
		return
	}
	sm.width = modalW

	// Build list items from valid transitions
	items := make([]modal.ListItem, len(sm.transitions))
	for i, status := range sm.transitions {
		items[i] = modal.ListItem{
			ID:    "status-" + status,
			Label: statusDisplayLabel(status),
		}
	}

	sm.m = modal.New("Change Status",
		modal.WithWidth(modalW),
		modal.WithPrimaryAction("change"),
	).
		AddSection(modal.Text("Current: " + statusDisplayLabel(sm.currentStatus))).
		AddSection(modal.Spacer()).
		AddSection(modal.Text("Move to:")).
		AddSection(modal.List("status-list", items, &sm.selectedIdx, modal.WithMaxVisible(5), modal.WithPerItemFocus())).
		AddSection(modal.Spacer()).
		AddSection(modal.When(
			func() bool { return sm.selectedStatus() == persephoneData.StatusBlocked },
			modal.InputWithLabel("block-reason", "Reason", &sm.blockInput),
		)).
		AddSection(modal.Spacer()).
		AddSection(modal.Buttons(
			modal.Btn(" Change ", "change"),
			modal.Btn(" Cancel ", "cancel"),
		))
}

// render returns the modal overlay string.
func (sm *statusModal) render(background string, screenW, screenH int) string {
	sm.buildModal(screenW)
	if sm.m == nil {
		return background
	}
	content := sm.m.Render(screenW, screenH, sm.mouseHandler)
	return ui.OverlayModal(background, content, screenW, screenH)
}

// handleKey processes keyboard input. Returns action and cmd.
func (sm *statusModal) handleKey(msg tea.KeyMsg) (action string, cmd tea.Cmd) {
	// Don't call buildModal here — render() already builds it each frame.
	// Calling buildModal(sm.width) would recompute modalW differently
	// (sm.width is modalW, not screenW), triggering a rebuild that creates
	// a fresh Modal with empty focusIDs, breaking all key routing.
	if sm.m == nil {
		return "", nil
	}

	// Track whether block input needs focus
	sm.needsBlockInput = sm.selectedStatus() == persephoneData.StatusBlocked

	action, cmd = sm.m.HandleKey(msg)
	return action, cmd
}

// consumesTextInput returns true when the block reason input is active.
func (sm *statusModal) consumesTextInput() bool {
	return sm.needsBlockInput && sm.m != nil && sm.m.FocusedID() == "block-reason"
}

// blockReason returns the entered block reason text.
func (sm *statusModal) blockReason() string {
	return sm.blockInput.Value()
}

// statusDisplayLabel maps status constants to human-readable labels.
func statusDisplayLabel(status string) string {
	switch status {
	case persephoneData.StatusOpen:
		return "Open"
	case persephoneData.StatusInProgress:
		return "In Progress"
	case persephoneData.StatusInReview:
		return "In Review"
	case persephoneData.StatusBlocked:
		return "Blocked"
	case persephoneData.StatusClosed:
		return "Closed"
	default:
		return status
	}
}
