package persephone

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/toddwbucy/hermes/internal/modal"
	"github.com/toddwbucy/hermes/internal/mouse"
	"github.com/toddwbucy/hermes/internal/ui"
)

// notesModal manages the note-entry textarea for a task.
type notesModal struct {
	taskKey      string
	m            *modal.Modal
	ta           textarea.Model
	mouseHandler *mouse.Handler
	width        int
}

// newNotesModal creates a notes modal for the given task.
func newNotesModal(taskKey string) *notesModal {
	ta := textarea.New()
	ta.Placeholder = "Add a note..."
	ta.SetHeight(5)
	ta.CharLimit = 2000

	return &notesModal{
		taskKey:      taskKey,
		ta:           ta,
		mouseHandler: mouse.NewHandler(),
	}
}

// buildModal lazily constructs the modal at the given screen width.
func (nm *notesModal) buildModal(screenWidth int) {
	modalW := ui.ModalWidthMedium + 10 // Slightly wider for textarea
	if modalW > screenWidth-4 {
		modalW = screenWidth - 4
	}
	if modalW < 40 {
		modalW = 40
	}

	if nm.m != nil && nm.width == modalW {
		return
	}
	nm.width = modalW

	nm.m = modal.New("Add Note",
		modal.WithWidth(modalW),
		modal.WithPrimaryAction("save"),
	).
		AddSection(modal.Textarea("note-content", &nm.ta, 5)).
		AddSection(modal.Spacer()).
		AddSection(modal.Text("ctrl+s to save")).
		AddSection(modal.Spacer()).
		AddSection(modal.Buttons(
			modal.Btn("  Save  ", "save"),
			modal.Btn(" Cancel ", "cancel"),
		))
}

// render returns the modal overlay string.
func (nm *notesModal) render(background string, screenW, screenH int) string {
	nm.buildModal(screenW)
	if nm.m == nil {
		return background
	}
	content := nm.m.Render(screenW, screenH, nm.mouseHandler)
	return ui.OverlayModal(background, content, screenW, screenH)
}

// handleKey processes keyboard input. Returns action and cmd.
func (nm *notesModal) handleKey(msg tea.KeyMsg) (action string, cmd tea.Cmd) {
	// Don't call buildModal here — render() already builds it each frame.
	// See status_modal.go for explanation of the width mismatch rebuild bug.
	if nm.m == nil {
		return "", nil
	}

	// Intercept ctrl+s as save — textarea eats Enter for newlines
	if msg.String() == "ctrl+s" {
		return "save", nil
	}

	action, cmd = nm.m.HandleKey(msg)
	return action, cmd
}

// consumesTextInput returns true when the textarea is focused.
func (nm *notesModal) consumesTextInput() bool {
	return nm.m != nil && nm.m.FocusedID() == "note-content"
}

// noteContent returns the trimmed textarea value.
func (nm *notesModal) noteContent() string {
	return strings.TrimSpace(nm.ta.Value())
}
