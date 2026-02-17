package persephone

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	persephoneData "github.com/toddwbucy/hermes/internal/persephone"
	"github.com/toddwbucy/hermes/internal/styles"
)

// detailModel shows full task info, sessions, handoffs, and edges.
type detailModel struct {
	task     *persephoneData.Task
	sessions []persephoneData.Session
	handoff  *persephoneData.Handoff
	edges    []persephoneData.Edge
	scroll   int
}

func newDetailModel() *detailModel {
	return &detailModel{}
}

func (d *detailModel) setTask(task *persephoneData.Task) {
	d.task = task
	d.sessions = nil
	d.handoff = nil
	d.edges = nil
	d.scroll = 0
}

func (d *detailModel) update(task *persephoneData.Task, sessions []persephoneData.Session, handoff *persephoneData.Handoff, edges []persephoneData.Edge) {
	if task != nil {
		d.task = task
	}
	d.sessions = sessions
	d.handoff = handoff
	d.edges = edges
}

func (d *detailModel) scrollDown() {
	d.scroll++
}

func (d *detailModel) scrollUp() {
	if d.scroll > 0 {
		d.scroll--
	}
}

func (d *detailModel) view(width, height int) string {
	if d.task == nil {
		return ""
	}

	t := d.task

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimary)
	labelStyle := lipgloss.NewStyle().Foreground(styles.TextMuted).Width(12)
	valueStyle := lipgloss.NewStyle().Foreground(styles.TextPrimary)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).MarginTop(1)

	var lines []string

	// Title
	lines = append(lines, headerStyle.Render(fmt.Sprintf("[%s] %s", t.Key, t.Title)))
	lines = append(lines, "")

	// Metadata
	statusHint := lipgloss.NewStyle().Foreground(styles.TextMuted).Render("  [s] [n]")
	lines = append(lines, labelStyle.Render("Status:")+" "+renderStatusBadge(t.Status)+statusHint)
	if t.Priority != "" {
		lines = append(lines, labelStyle.Render("Priority:")+" "+valueStyle.Render(t.Priority))
	}
	if t.Type != "" {
		lines = append(lines, labelStyle.Render("Type:")+" "+valueStyle.Render(t.Type))
	}
	if len(t.Labels) > 0 {
		lines = append(lines, labelStyle.Render("Labels:")+" "+valueStyle.Render(strings.Join(t.Labels, ", ")))
	}
	if t.BlockReason != "" {
		lines = append(lines, labelStyle.Render("Blocked:")+" "+lipgloss.NewStyle().Foreground(styles.Error).Render(t.BlockReason))
	}

	// Description
	if t.Description != "" {
		lines = append(lines, sectionStyle.Render("Description"))
		lines = append(lines, valueStyle.Render(t.Description))
	}

	// Acceptance criteria
	if t.Acceptance != "" {
		lines = append(lines, sectionStyle.Render("Acceptance Criteria"))
		lines = append(lines, valueStyle.Render(t.Acceptance))
	}

	// Sessions
	if len(d.sessions) > 0 {
		lines = append(lines, sectionStyle.Render(fmt.Sprintf("Sessions (%d)", len(d.sessions))))
		for _, s := range d.sessions {
			agent := s.AgentType
			if agent == "" {
				agent = "unknown"
			}
			branch := s.Branch
			if branch == "" {
				branch = "-"
			}
			ended := "active"
			if s.EndedAt != nil {
				ended = s.EndedAt.Format("2006-01-02 15:04")
			}
			lines = append(lines, fmt.Sprintf("  %s  agent=%s  branch=%s  ended=%s",
				lipgloss.NewStyle().Foreground(styles.TextMuted).Render(s.Key),
				agent, branch, ended))
		}
	}

	// Dependencies (blocked_by edges)
	blockedBy := filterEdges(d.edges, persephoneData.EdgeBlockedBy)
	if len(blockedBy) > 0 {
		lines = append(lines, sectionStyle.Render("Blocked By"))
		for _, e := range blockedBy {
			// Extract key from full _id (e.g., "persephone_tasks/task_xxx" → "task_xxx")
			fromKey := e.From
			if idx := strings.LastIndex(e.From, "/"); idx >= 0 {
				fromKey = e.From[idx+1:]
			}
			lines = append(lines, fmt.Sprintf("  → %s", fromKey))
		}
	}

	// Latest handoff
	if d.handoff != nil {
		lines = append(lines, sectionStyle.Render("Latest Handoff"))
		if len(d.handoff.Done) > 0 {
			lines = append(lines, labelStyle.Render("  Done:"))
			for _, item := range d.handoff.Done {
				lines = append(lines, "    ✓ "+item)
			}
		}
		if len(d.handoff.Remaining) > 0 {
			lines = append(lines, labelStyle.Render("  Remaining:"))
			for _, item := range d.handoff.Remaining {
				lines = append(lines, "    ○ "+item)
			}
		}
		if len(d.handoff.Decisions) > 0 {
			lines = append(lines, labelStyle.Render("  Decisions:"))
			for _, item := range d.handoff.Decisions {
				lines = append(lines, "    • "+item)
			}
		}
		if len(d.handoff.Uncertain) > 0 {
			lines = append(lines, labelStyle.Render("  Uncertain:"))
			for _, item := range d.handoff.Uncertain {
				lines = append(lines, "    ? "+item)
			}
		}
		if d.handoff.GitBranch != "" {
			lines = append(lines, fmt.Sprintf("  git: %s @ %s", d.handoff.GitBranch, truncate(d.handoff.GitSHA, 8)))
		}
	}

	// Notes
	noteHint := lipgloss.NewStyle().Foreground(styles.TextMuted).Render("  [n]")
	if len(t.Notes) > 0 {
		lines = append(lines, sectionStyle.Render(fmt.Sprintf("Notes (%d)", len(t.Notes)))+noteHint)
		noteTimeStyle := lipgloss.NewStyle().Foreground(styles.TextMuted)
		// Content area: width minus padding (2 sides * 1 char) and indent (2 chars)
		wrapWidth := width - 6
		if wrapWidth < 20 {
			wrapWidth = 20
		}
		for _, n := range t.Notes {
			ts := n.CreatedAt.Format("2006-01-02 15:04")
			lines = append(lines, noteTimeStyle.Render(fmt.Sprintf("  %s · %s", ts, n.Author)))
			wrapped := wrapNoteContent(n.Content, wrapWidth)
			for _, wl := range strings.Split(wrapped, "\n") {
				lines = append(lines, "  "+valueStyle.Render(wl))
			}
			lines = append(lines, "")
		}
	} else {
		lines = append(lines, sectionStyle.Render("Notes")+noteHint)
	}

	// Join all lines
	content := strings.Join(lines, "\n")

	// Apply scrolling
	allLines := strings.Split(content, "\n")
	if d.scroll > len(allLines)-height {
		d.scroll = len(allLines) - height
	}
	if d.scroll < 0 {
		d.scroll = 0
	}
	endIdx := d.scroll + height
	if endIdx > len(allLines) {
		endIdx = len(allLines)
	}
	visible := allLines[d.scroll:endIdx]

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(visible, "\n"))
}

// renderStatusBadge renders a colored status label.
func renderStatusBadge(status string) string {
	color, ok := columnColors[status]
	if !ok {
		color = styles.TextSecondary
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(status)
}

// filterEdges returns edges matching the given type where the task is the target.
func filterEdges(edges []persephoneData.Edge, edgeType string) []persephoneData.Edge {
	var result []persephoneData.Edge
	for _, e := range edges {
		if e.Type == edgeType {
			result = append(result, e)
		}
	}
	return result
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// wrapNoteContent performs greedy word-wrap, preserving explicit newlines.
func wrapNoteContent(content string, maxWidth int) string {
	if maxWidth < 1 {
		maxWidth = 1
	}
	var out strings.Builder
	for i, paragraph := range strings.Split(content, "\n") {
		if i > 0 {
			out.WriteByte('\n')
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}
		lineLen := 0
		for j, w := range words {
			wLen := runewidth.StringWidth(w)
			if j == 0 {
				out.WriteString(w)
				lineLen = wLen
			} else if lineLen+1+wLen > maxWidth {
				out.WriteByte('\n')
				out.WriteString(w)
				lineLen = wLen
			} else {
				out.WriteByte(' ')
				out.WriteString(w)
				lineLen += 1 + wLen
			}
		}
	}
	return out.String()
}
