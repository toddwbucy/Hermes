package conversations

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toddwbucy/hermes/internal/modal"
	"github.com/toddwbucy/hermes/internal/styles"
)

// renderInsightModal renders the insight extraction modal as an overlay string.
func renderInsightModal(state *insightModalState, sessionName string, width, height int) string {
	if state == nil {
		return ""
	}

	modalWidth := width - 8
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > 90 {
		modalWidth = 90
	}

	effectiveHeight := height - 4
	if effectiveHeight < 10 {
		effectiveHeight = 10
	}

	title := "Insights"
	if sessionName != "" {
		title = "Insights: " + truncateStr(sessionName, 40)
	}

	if len(state.insights) == 0 {
		m := modal.New(title,
			modal.WithWidth(modalWidth),
			modal.WithHints(false),
		).
			AddSection(modal.Text("No insights found in this conversation.")).
			AddSection(modal.Spacer()).
			AddSection(modal.Text(styles.Muted.Render("Try a conversation that contains ★ Insight blocks, key observations, or design decisions."))).
			AddSection(modal.Spacer()).
			AddSection(modal.Buttons(
				modal.Btn("Close", "cancel"),
			))

		return m.Render(width, effectiveHeight, nil)
	}

	// Build the insight list and detail sections
	selectedCount := countSelected(state.insights)
	footer := fmt.Sprintf(" %d/%d selected  ·  space:toggle  a:all  enter:create  esc:close",
		selectedCount, len(state.insights))

	m := modal.New(title,
		modal.WithWidth(modalWidth),
		modal.WithHints(false),
		modal.WithCustomFooter(footer),
	).
		AddSection(insightListSection(state, effectiveHeight-10, modalWidth-6)).
		AddSection(modal.Spacer()).
		AddSection(insightDetailSection(state, modalWidth-6)).
		AddSection(modal.Spacer()).
		AddSection(modal.Buttons(
			modal.Btn(fmt.Sprintf("Create Tasks (%d)", selectedCount), "create", modal.BtnPrimary()),
			modal.Btn("Cancel", "cancel"),
		))

	return m.Render(width, effectiveHeight, nil)
}

// insightListSection creates a custom section rendering the scrollable insight list with checkboxes.
func insightListSection(state *insightModalState, maxHeight, contentWidth int) modal.Section {
	return modal.Custom(
		func(cw int, focusID, hoverID string) modal.RenderedSection {
			if len(state.insights) == 0 {
				return modal.RenderedSection{Content: styles.Muted.Render("(no insights)")}
			}

			visibleCount := maxHeight
			if visibleCount < 3 {
				visibleCount = 3
			}
			if visibleCount > len(state.insights) {
				visibleCount = len(state.insights)
			}

			// Adjust scroll offset to keep cursor visible
			if state.cursor < 0 {
				state.cursor = 0
			}
			if state.cursor >= len(state.insights) {
				state.cursor = len(state.insights) - 1
			}

			scrollOff := 0
			if state.cursor >= visibleCount {
				scrollOff = state.cursor - visibleCount + 1
			}

			maxScroll := len(state.insights) - visibleCount
			if maxScroll < 0 {
				maxScroll = 0
			}
			if scrollOff > maxScroll {
				scrollOff = maxScroll
			}

			var sb strings.Builder
			for i := 0; i < visibleCount; i++ {
				idx := scrollOff + i
				if idx >= len(state.insights) {
					break
				}
				ins := state.insights[idx]
				isCursor := idx == state.cursor

				// Checkbox
				box := "[ ]"
				if ins.Selected {
					box = "[x]"
				}

				// Source badge
				badge := fmt.Sprintf("[%s]", ins.Source.Badge())

				// Truncated text preview
				maxTextW := contentWidth - len(box) - len(badge) - 6 // spacing
				if maxTextW < 10 {
					maxTextW = 10
				}
				preview := truncateStr(firstLine(ins.Text), maxTextW)

				line := fmt.Sprintf("%s %s %s", box, badge, preview)

				if isCursor {
					line = styles.ListItemFocused.Render("▸ " + line)
				} else {
					line = "  " + line
				}

				if i > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(line)
			}

			content := sb.String()
			if scrollOff > 0 {
				content = styles.Muted.Render("↑ more above") + "\n" + content
			}
			if scrollOff+visibleCount < len(state.insights) {
				content = content + "\n" + styles.Muted.Render("↓ more below")
			}

			return modal.RenderedSection{Content: content}
		},
		nil,
	)
}

// insightDetailSection shows the full text of the currently highlighted insight.
func insightDetailSection(state *insightModalState, contentWidth int) modal.Section {
	return modal.Custom(
		func(cw int, focusID, hoverID string) modal.RenderedSection {
			if len(state.insights) == 0 || state.cursor >= len(state.insights) {
				return modal.RenderedSection{}
			}

			ins := state.insights[state.cursor]
			headerStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(styles.TextSecondary)

			header := headerStyle.Render(fmt.Sprintf("[%s] Turn %d", ins.Source.Badge(), ins.TurnIndex+1))

			// Wrap the full text
			text := ins.Text
			wrapped := wrapToWidth(text, contentWidth)

			// Limit detail height
			lines := strings.Split(wrapped, "\n")
			if len(lines) > 6 {
				lines = lines[:6]
				lines = append(lines, styles.Muted.Render("..."))
			}

			content := header + "\n" + strings.Join(lines, "\n")
			return modal.RenderedSection{Content: content}
		},
		nil,
	)
}

// countSelected returns how many insights are selected.
func countSelected(insights []Insight) int {
	n := 0
	for _, ins := range insights {
		if ins.Selected {
			n++
		}
	}
	return n
}

// truncateStr truncates a string to maxLen runes, appending "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// firstLine returns the first line of text, stripped of leading markdown.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	// Strip leading markdown bullet
	s = strings.TrimLeft(s, "-*• ")
	return strings.TrimSpace(s)
}

// wrapToWidth wraps text to fit within width, respecting existing newlines.
func wrapToWidth(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		if ansi.StringWidth(line) <= width {
			result = append(result, line)
			continue
		}
		words := strings.Fields(line)
		var current string
		for _, w := range words {
			if current == "" {
				current = w
			} else if ansi.StringWidth(current+" "+w) <= width {
				current += " " + w
			} else {
				result = append(result, current)
				current = w
			}
		}
		if current != "" {
			result = append(result, current)
		}
	}
	return strings.Join(result, "\n")
}
