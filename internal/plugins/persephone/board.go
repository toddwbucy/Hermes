package persephone

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwbucy/hermes/internal/mouse"
	persephoneData "github.com/toddwbucy/hermes/internal/persephone"
	"github.com/toddwbucy/hermes/internal/styles"
)

// boardColumns defines the column order for the kanban board.
var boardColumns = []string{
	persephoneData.StatusOpen,
	persephoneData.StatusInProgress,
	persephoneData.StatusInReview,
	persephoneData.StatusBlocked,
	persephoneData.StatusClosed,
}

// columnLabels maps status to display label.
var columnLabels = map[string]string{
	persephoneData.StatusOpen:       "Open",
	persephoneData.StatusInProgress: "In Progress",
	persephoneData.StatusInReview:   "In Review",
	persephoneData.StatusBlocked:    "Blocked",
	persephoneData.StatusClosed:     "Closed",
}

// columnColors maps status to header color.
var columnColors = map[string]lipgloss.Color{
	persephoneData.StatusOpen:       styles.Info,
	persephoneData.StatusInProgress: styles.Primary,
	persephoneData.StatusInReview:   styles.Warning,
	persephoneData.StatusBlocked:    styles.Error,
	persephoneData.StatusClosed:     styles.Success,
}

// boardModel holds the kanban board state.
type boardModel struct {
	columns   map[string][]persephoneData.Task
	colIdx    int // Active column
	rowIdx    int // Selected row within column
	scrollTop map[string]int
}

func newBoardModel() *boardModel {
	return &boardModel{
		columns:   make(map[string][]persephoneData.Task),
		scrollTop: make(map[string]int),
	}
}

func (b *boardModel) updateTasks(tasks []persephoneData.Task) {
	b.columns = make(map[string][]persephoneData.Task)
	for _, t := range tasks {
		b.columns[t.Status] = append(b.columns[t.Status], t)
	}
}

func (b *boardModel) totalTasks() int {
	total := 0
	for _, tasks := range b.columns {
		total += len(tasks)
	}
	return total
}

func (b *boardModel) activeColumn() string {
	if b.colIdx < 0 || b.colIdx >= len(boardColumns) {
		return boardColumns[0]
	}
	return boardColumns[b.colIdx]
}

func (b *boardModel) selectedTask() *persephoneData.Task {
	col := b.activeColumn()
	tasks := b.columns[col]
	if b.rowIdx < 0 || b.rowIdx >= len(tasks) {
		return nil
	}
	return &tasks[b.rowIdx]
}

func (b *boardModel) moveUp() {
	if b.rowIdx > 0 {
		b.rowIdx--
	}
}

func (b *boardModel) moveDown() {
	col := b.activeColumn()
	if b.rowIdx < len(b.columns[col])-1 {
		b.rowIdx++
	}
}

func (b *boardModel) moveLeft() {
	if b.colIdx > 0 {
		b.colIdx--
		b.clampRow()
	}
}

func (b *boardModel) moveRight() {
	if b.colIdx < len(boardColumns)-1 {
		b.colIdx++
		b.clampRow()
	}
}

// selectByIndex selects a task by its flat index across all columns.
// Returns the task if found, nil otherwise.
func (b *boardModel) selectByIndex(flatIdx int) *persephoneData.Task {
	offset := 0
	for i, status := range boardColumns {
		tasks := b.columns[status]
		if flatIdx >= offset && flatIdx < offset+len(tasks) {
			b.colIdx = i
			b.rowIdx = flatIdx - offset
			return &tasks[b.rowIdx]
		}
		offset += len(tasks)
	}
	return nil
}

func (b *boardModel) clampRow() {
	col := b.activeColumn()
	max := len(b.columns[col]) - 1
	if max < 0 {
		max = 0
	}
	if b.rowIdx > max {
		b.rowIdx = max
	}
}

func (b *boardModel) view(width, height int, mh *mouse.Handler) string {
	if width < 20 || height < 5 {
		return ""
	}

	// Clear and rebuild hit regions each frame
	if mh != nil {
		mh.Clear()
		mh.HitMap.AddRect(regionBoard, 0, 0, width, height, nil)
	}

	numCols := len(boardColumns)
	colWidth := (width - numCols - 1) / numCols
	if colWidth < 12 {
		colWidth = 12
	}

	// Calculate visible task rows (height minus header and borders)
	visibleRows := height - 4
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Flat index counter for hit region data payload
	flatIdx := 0

	// Each rendered column occupies: 1 (border-left) + colWidth + 1 (border-right) = colWidth + 2
	renderedColWidth := colWidth + 2

	var columnViews []string
	for i, status := range boardColumns {
		tasks := b.columns[status]
		isActive := i == b.colIdx

		// Column header
		label := columnLabels[status]
		count := len(tasks)
		headerText := fmt.Sprintf(" %s (%d) ", label, count)

		headerColor := columnColors[status]
		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(headerColor).
			Width(colWidth).
			Align(lipgloss.Center)
		if isActive {
			headerStyle = headerStyle.Underline(true)
		}
		header := headerStyle.Render(headerText)

		// Scroll management
		scrollKey := status
		scrollTop := b.scrollTop[scrollKey]

		// Ensure selected row is visible when this is the active column
		if isActive {
			if b.rowIdx < scrollTop {
				scrollTop = b.rowIdx
			}
			if b.rowIdx >= scrollTop+visibleRows {
				scrollTop = b.rowIdx - visibleRows + 1
			}
			b.scrollTop[scrollKey] = scrollTop
		}

		// X offset for this column in the final joined output
		colX := i * renderedColWidth

		// Task cards
		var cards []string
		endIdx := scrollTop + visibleRows
		if endIdx > len(tasks) {
			endIdx = len(tasks)
		}

		// Track flat offset for tasks before visible window
		colFlatBase := flatIdx
		for j := scrollTop; j < endIdx; j++ {
			t := tasks[j]
			isSelected := isActive && j == b.rowIdx
			cards = append(cards, renderTaskCard(t, colWidth, isSelected))

			// Register hit region for this card:
			// Y = 2 (top border + header) + (j - scrollTop) * 2 (each card is 2 lines)
			if mh != nil {
				cardY := 2 + (j-scrollTop)*2
				mh.HitMap.AddRect(regionTaskCard, colX+1, cardY, colWidth, 2, colFlatBase+j)
			}
		}

		flatIdx += len(tasks)

		// Pad with empty rows
		for len(cards) < visibleRows {
			cards = append(cards, lipgloss.NewStyle().Width(colWidth).Render(""))
		}

		col := lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(cards, "\n"))

		// Column border
		borderColor := styles.BorderNormal
		if isActive {
			borderColor = styles.BorderActive
		}
		colStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(colWidth)
		columnViews = append(columnViews, colStyle.Render(col))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, columnViews...)
}

// renderTaskCard renders a single task as a card.
func renderTaskCard(t persephoneData.Task, width int, selected bool) string {
	// Truncate key to short form
	key := t.Key
	if len(key) > 12 {
		key = key[:12]
	}

	// Build card text
	title := t.Title
	maxTitle := width - 4
	if maxTitle < 10 {
		maxTitle = 10
	}
	if len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	// Priority/type badge
	badge := ""
	if t.Priority == persephoneData.PriorityHigh || t.Priority == persephoneData.PriorityCritical {
		badge = "!"
	}
	if t.Type == persephoneData.TypeBug {
		badge = "B"
	}

	line1 := fmt.Sprintf("[%s]", key)
	if badge != "" {
		line1 = fmt.Sprintf("[%s] %s", key, badge)
	}

	cardStyle := lipgloss.NewStyle().Width(width - 2).Padding(0, 1)

	if selected {
		cardStyle = cardStyle.
			Background(styles.BgTertiary).
			Foreground(styles.TextSelectionColor).
			Bold(true)
	} else {
		cardStyle = cardStyle.Foreground(styles.TextSecondary)
	}

	keyStyle := lipgloss.NewStyle().Foreground(styles.TextMuted)
	if selected {
		keyStyle = keyStyle.Foreground(styles.TextSelectionColor).Background(styles.BgTertiary)
	}

	return cardStyle.Render(keyStyle.Render(line1) + "\n" + title)
}
