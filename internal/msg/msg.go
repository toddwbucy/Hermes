package msg

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ToastMsg displays a temporary message.
type ToastMsg struct {
	Message  string
	Duration time.Duration
	IsError  bool // true for error toasts (red), false for success (green)
}

// ShowToast returns a command to show a toast message.
func ShowToast(message string, duration time.Duration) tea.Cmd {
	return func() tea.Msg {
		return ToastMsg{
			Message:  message,
			Duration: duration,
		}
	}
}

// InsightTask is a single insight to be created as a Persephone task.
type InsightTask struct {
	Title       string // First ~80 chars of insight text
	Description string // Full insight text + source reference
}

// CreateInsightTasksMsg is emitted by the conversations plugin to request
// Persephone task creation from extracted insights. Broadcast to all plugins.
type CreateInsightTasksMsg struct {
	Tasks       []InsightTask
	SessionName string
}

// InsightTasksCreatedMsg is emitted by the Persephone plugin after processing
// a CreateInsightTasksMsg. Broadcast back to conversations plugin.
type InsightTasksCreatedMsg struct {
	Count int
	Err   error
}
