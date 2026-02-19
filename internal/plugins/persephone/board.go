package persephone

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwbucy/hermes/internal/mouse"
	persephoneData "github.com/toddwbucy/hermes/internal/persephone"
	"github.com/toddwbucy/hermes/internal/styles"
)

// SortMode defines how tasks are ordered within columns.
type SortMode int

const (
	SortByUpdated  SortMode = iota // updated_at DESC (default)
	SortByCreated                  // created_at DESC
	SortByTitle                    // title ASC (alphabetical)
	SortByPriority                 // critical > high > medium > low
	sortModeCount                  // sentinel for modular cycling
)

// Label returns a short display label for the sort mode.
func (s SortMode) Label() string {
	switch s {
	case SortByUpdated:
		return "updated"
	case SortByCreated:
		return "created"
	case SortByTitle:
		return "title"
	case SortByPriority:
		return "priority"
	default:
		return "updated"
	}
}

// Next cycles to the next sort mode.
func (s SortMode) Next() SortMode { return (s + 1) % sortModeCount }

// priorityRank maps priority strings to sort rank (higher = more important = sorts first).
var priorityRank = map[string]int{
	persephoneData.PriorityCritical: 4,
	persephoneData.PriorityHigh:     3,
	persephoneData.PriorityMedium:   2,
	persephoneData.PriorityLow:      1,
}

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
	sortMode  SortMode
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
	b.sortColumns()
}

// sortColumns applies the current sort mode to all columns.
func (b *boardModel) sortColumns() {
	for status := range b.columns {
		tasks := b.columns[status]
		switch b.sortMode {
		case SortByUpdated:
			sort.Slice(tasks, func(i, j int) bool {
				return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
			})
		case SortByCreated:
			sort.Slice(tasks, func(i, j int) bool {
				return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
			})
		case SortByTitle:
			sort.Slice(tasks, func(i, j int) bool {
				return tasks[i].Title < tasks[j].Title
			})
		case SortByPriority:
			sort.Slice(tasks, func(i, j int) bool {
				return priorityRank[tasks[i].Priority] > priorityRank[tasks[j].Priority]
			})
		}
	}
}

// cycleSort advances to the next sort mode and re-sorts all columns.
func (b *boardModel) cycleSort() {
	b.sortMode = b.sortMode.Next()
	b.sortColumns()
	// Reset scroll positions so the user sees the top of each column
	for k := range b.scrollTop {
		b.scrollTop[k] = 0
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

	// Unified grid layout: ╭─┬─┬─╮ ... ╰─┴─┴─╯
	// Total = 1 (left border) + sum(colWidths) + (numCols-1) separators + 1 (right border)
	//       = sum(colWidths) + numCols + 1
	// So sum(colWidths) = width - numCols - 1
	available := width - numCols - 1
	baseColWidth := available / numCols
	if baseColWidth < 10 {
		baseColWidth = 10
	}
	colWidthRemainder := available - baseColWidth*numCols

	colWidths := make([]int, numCols)
	for i := range colWidths {
		colWidths[i] = baseColWidth
		if i < colWidthRemainder {
			colWidths[i]++
		}
	}

	// Calculate visible task cards. Each card is 2 lines tall.
	// Subtract 4 lines for header + top/bottom borders + separator, then divide by card height.
	cardHeight := 2
	visibleCards := (height - 4) / cardHeight
	if visibleCards < 1 {
		visibleCards = 1
	}

	// Flat index counter for hit region data payload
	flatIdx := 0

	// Track cumulative X offset for mouse hit regions
	colX := 0

	// Build per-column content as line arrays (no borders yet)
	contentLines := make([][]string, numCols)
	for i, status := range boardColumns {
		colWidth := colWidths[i]
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

		// Ensure selected row is visible when this is the active column.
		// We must account for scroll indicators stealing card slots:
		// when the cursor is near the bottom, a "↓ N more" indicator
		// will consume one slot, so the actual visible window is smaller
		// than visibleCards. We iterate to converge (at most 2 passes)
		// because adjusting scrollTop can toggle indicators on/off.
		if isActive {
			for range 2 {
				hasMoreTmp := len(tasks) > visibleCards
				upTmp := hasMoreTmp && scrollTop > 0
				downTmp := hasMoreTmp && scrollTop+visibleCards < len(tasks)
				effective := visibleCards
				if upTmp {
					effective--
				}
				if downTmp {
					effective--
				}
				if effective < 1 {
					effective = 1
				}
				if b.rowIdx < scrollTop {
					scrollTop = b.rowIdx
				}
				if b.rowIdx >= scrollTop+effective {
					scrollTop = b.rowIdx - effective + 1
				}
			}
			b.scrollTop[scrollKey] = scrollTop
		}

		// Determine scroll indicators (final, using settled scrollTop)
		hasMore := len(tasks) > visibleCards
		showUpIndicator := hasMore && scrollTop > 0
		showDownIndicator := hasMore && scrollTop+visibleCards < len(tasks)

		displayCards := visibleCards
		if showUpIndicator {
			displayCards--
		}
		if showDownIndicator {
			displayCards--
		}
		if displayCards < 1 {
			displayCards = 1
		}

		// Task cards
		var cards []string

		if showUpIndicator {
			upText := fmt.Sprintf("↑ %d more", scrollTop)
			upStyle := lipgloss.NewStyle().Width(colWidth).Foreground(styles.TextMuted).Align(lipgloss.Center)
			cards = append(cards, upStyle.Render(upText+"\n"))
		}

		endIdx := scrollTop + displayCards
		if endIdx > len(tasks) {
			endIdx = len(tasks)
		}

		colFlatBase := flatIdx
		cardYOffset := 0
		if showUpIndicator {
			cardYOffset = cardHeight
		}
		for j := scrollTop; j < endIdx; j++ {
			t := tasks[j]
			isSelected := isActive && j == b.rowIdx
			cards = append(cards, renderTaskCard(t, colWidth, isSelected))

			if mh != nil {
				cardY := 2 + cardYOffset + (j-scrollTop)*cardHeight
				mh.HitMap.AddRect(regionTaskCard, colX+1, cardY, colWidth, cardHeight, colFlatBase+j)
			}
		}

		flatIdx += len(tasks)

		if showDownIndicator {
			remaining := len(tasks) - endIdx
			downText := fmt.Sprintf("↓ %d more", remaining)
			downStyle := lipgloss.NewStyle().Width(colWidth).Foreground(styles.TextMuted).Align(lipgloss.Center)
			cards = append(cards, downStyle.Render("\n"+downText))
		}

		for len(cards) < visibleCards {
			cards = append(cards, lipgloss.NewStyle().Width(colWidth).Render("\n"))
		}

		col := lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(cards, "\n"))
		contentLines[i] = strings.Split(col, "\n")

		// Advance X offset for next column's hit regions
		colX += colWidth + 1 // content + one separator
	}

	// Determine content height (should be consistent across columns)
	contentHeight := 0
	for _, lines := range contentLines {
		if len(lines) > contentHeight {
			contentHeight = len(lines)
		}
	}
	// Pad shorter columns to match
	for i, lines := range contentLines {
		for len(lines) < contentHeight {
			lines = append(lines, strings.Repeat(" ", colWidths[i]))
		}
		contentLines[i] = lines
	}

	// Build unified grid with shared borders.
	// The active column's surrounding borders use the active color.
	normalBorder := lipgloss.NewStyle().Foreground(styles.BorderNormal)
	activeBorder := lipgloss.NewStyle().Foreground(styles.BorderActive)
	activeCol := b.colIdx

	// borderStyle returns the style for a separator or segment adjacent to column i.
	// A separator between columns i and i+1 is "active" if either side is the active column.
	// A segment spanning column i is "active" if i is the active column.
	segStyle := func(col int) lipgloss.Style {
		if col == activeCol {
			return activeBorder
		}
		return normalBorder
	}
	sepStyle := func(left, right int) lipgloss.Style {
		if left == activeCol || right == activeCol {
			return activeBorder
		}
		return normalBorder
	}

	var sb strings.Builder

	// Top border: ╭───┬───┬───╮
	sb.WriteString(segStyle(0).Render("╭"))
	for i := 0; i < numCols; i++ {
		sb.WriteString(segStyle(i).Render(strings.Repeat("─", colWidths[i])))
		if i < numCols-1 {
			sb.WriteString(sepStyle(i, i+1).Render("┬"))
		}
	}
	sb.WriteString(segStyle(numCols - 1).Render("╮"))
	sb.WriteByte('\n')

	// Content rows: │content│content│
	for row := 0; row < contentHeight; row++ {
		sb.WriteString(segStyle(0).Render("│"))
		for i := 0; i < numCols; i++ {
			line := contentLines[i][row]
			lineW := lipgloss.Width(line)
			if lineW < colWidths[i] {
				line += strings.Repeat(" ", colWidths[i]-lineW)
			}
			sb.WriteString(line)
			if i < numCols-1 {
				sb.WriteString(sepStyle(i, i+1).Render("│"))
			}
		}
		sb.WriteString(segStyle(numCols - 1).Render("│"))
		if row < contentHeight-1 {
			sb.WriteByte('\n')
		}
	}
	sb.WriteByte('\n')

	// Bottom border: ╰───┴───┴───╯
	sb.WriteString(segStyle(0).Render("╰"))
	for i := 0; i < numCols; i++ {
		sb.WriteString(segStyle(i).Render(strings.Repeat("─", colWidths[i])))
		if i < numCols-1 {
			sb.WriteString(sepStyle(i, i+1).Render("┴"))
		}
	}
	sb.WriteString(segStyle(numCols - 1).Render("╯"))

	return sb.String()
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
