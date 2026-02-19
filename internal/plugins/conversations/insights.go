package conversations

import (
	"regexp"
	"strings"
)

// InsightSource classifies how an insight was detected.
type InsightSource string

const (
	SourceInsightBlock InsightSource = "insight-block"
	SourceObservation  InsightSource = "key-observation"
	SourceDecision     InsightSource = "decision"
	SourcePattern      InsightSource = "pattern"
)

// Insight represents an extracted insight from a conversation.
type Insight struct {
	Text      string        // The extracted insight text
	Source    InsightSource // How it was detected
	TurnIndex int           // Which turn it came from
	Selected  bool          // User toggle for task creation
}

// insightModalState holds state for the insight extraction modal.
type insightModalState struct {
	insights  []Insight
	cursor    int
	allSelect bool // toggle for select-all
}

// Badge returns a short label for the insight source.
func (s InsightSource) Badge() string {
	switch s {
	case SourceInsightBlock:
		return "★"
	case SourceObservation:
		return "OBS"
	case SourceDecision:
		return "DEC"
	case SourcePattern:
		return "PAT"
	default:
		return "?"
	}
}

// Regex patterns for insight extraction.
var (
	// ★ Insight block delimiter (the ───── lines from explanatory mode)
	insightDelimiterRe = regexp.MustCompile(`^` + "`" + `?[─━]{5,}` + "`" + `?$`)
	insightHeaderRe    = regexp.MustCompile(`(?i)★\s*Insight`)

	// Key observation / important patterns (bold-prefixed)
	keyPhraseRe = regexp.MustCompile(`(?i)\*\*(Key\s+(?:observation|insight|takeaway|finding)|Important|Critical\s+finding|Design\s+decision|Architectural\s+decision)\*\*`)

	// Blockquote note patterns
	blockquoteNoteRe = regexp.MustCompile(`(?i)^>\s*\*\*(Note|Important|Warning)\*?\*?:`)

	// Section header patterns
	sectionHeaderRe = regexp.MustCompile(`(?i)^#{1,3}\s+(Observations?|Decisions?|Insights?|Key\s+Takeaways?)`)

	// Inline signal phrases
	signalPhraseRe = regexp.MustCompile(`(?i)(critical finding|key takeaway|design decision|architectural decision)`)
)

// extractInsights scans assistant turns for insight patterns.
func extractInsights(turns []Turn) []Insight {
	var insights []Insight
	seen := make(map[string]bool) // deduplicate by text

	for turnIdx, turn := range turns {
		if turn.Role != "assistant" {
			continue
		}
		for _, msg := range turn.Messages {
			for _, block := range msg.ContentBlocks {
				if block.Type != "text" || block.Text == "" {
					continue
				}
				extracted := extractFromText(block.Text, turnIdx)
				for _, ins := range extracted {
					key := strings.TrimSpace(ins.Text)
					if key == "" || seen[key] {
						continue
					}
					seen[key] = true
					insights = append(insights, ins)
				}
			}
		}
	}
	return insights
}

// extractFromText applies heuristics to a single text block.
func extractFromText(text string, turnIdx int) []Insight {
	var insights []Insight
	lines := strings.Split(text, "\n")

	// Pass 1: Extract ★ Insight blocks (delimited by ───── lines)
	insights = append(insights, extractInsightBlocks(lines, turnIdx)...)

	// Pass 2: Extract key-phrase lines
	insights = append(insights, extractKeyPhrases(lines, turnIdx)...)

	// Pass 3: Extract blockquote notes
	insights = append(insights, extractBlockquoteNotes(lines, turnIdx)...)

	// Pass 4: Extract section-headed bullet points
	insights = append(insights, extractSectionBullets(lines, turnIdx)...)

	// Pass 5: Extract signal-phrase sentences
	insights = append(insights, extractSignalPhrases(lines, turnIdx)...)

	return insights
}

// extractInsightBlocks finds ★ Insight blocks bounded by ───── delimiters.
func extractInsightBlocks(lines []string, turnIdx int) []Insight {
	var insights []Insight

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		// Look for ★ Insight header or delimiter start
		if insightHeaderRe.MatchString(line) {
			// Find the content between delimiters
			contentStart := i + 1
			// Skip to first delimiter if header is on same line
			for contentStart < len(lines) {
				if insightDelimiterRe.MatchString(strings.TrimSpace(lines[contentStart])) {
					contentStart++
					break
				}
				contentStart++
			}
			// Collect until closing delimiter
			var contentLines []string
			j := contentStart
			for j < len(lines) {
				if insightDelimiterRe.MatchString(strings.TrimSpace(lines[j])) {
					break
				}
				contentLines = append(contentLines, lines[j])
				j++
			}
			if len(contentLines) > 0 {
				text := strings.TrimSpace(strings.Join(contentLines, "\n"))
				if text != "" {
					insights = append(insights, Insight{
						Text:      text,
						Source:    SourceInsightBlock,
						TurnIndex: turnIdx,
					})
				}
			}
			i = j + 1
			continue
		}
		i++
	}
	return insights
}

// extractKeyPhrases finds lines with bold key-phrase patterns.
func extractKeyPhrases(lines []string, turnIdx int) []Insight {
	var insights []Insight
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !keyPhraseRe.MatchString(trimmed) {
			continue
		}
		// Collect this line and any continuation lines (non-empty, non-header)
		text := trimmed
		for j := i + 1; j < len(lines) && j <= i+3; j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" || strings.HasPrefix(next, "#") || strings.HasPrefix(next, "**") {
				break
			}
			text += " " + next
		}
		insights = append(insights, Insight{
			Text:      text,
			Source:    SourceObservation,
			TurnIndex: turnIdx,
		})
	}
	return insights
}

// extractBlockquoteNotes finds > **Note:** / > **Important:** patterns.
func extractBlockquoteNotes(lines []string, turnIdx int) []Insight {
	var insights []Insight
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !blockquoteNoteRe.MatchString(trimmed) {
			continue
		}
		// Collect continued blockquote lines
		text := strings.TrimPrefix(trimmed, "> ")
		for j := i + 1; j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if !strings.HasPrefix(next, ">") || next == ">" {
				break
			}
			text += " " + strings.TrimPrefix(next, "> ")
		}
		insights = append(insights, Insight{
			Text:      text,
			Source:    SourceObservation,
			TurnIndex: turnIdx,
		})
	}
	return insights
}

// extractSectionBullets finds bullet points under ## Observations/Decisions/Insights headers.
func extractSectionBullets(lines []string, turnIdx int) []Insight {
	var insights []Insight
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !sectionHeaderRe.MatchString(trimmed) {
			i++
			continue
		}
		// Determine source from header
		source := SourcePattern
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "decision") {
			source = SourceDecision
		} else if strings.Contains(lower, "observation") || strings.Contains(lower, "insight") || strings.Contains(lower, "takeaway") {
			source = SourceObservation
		}

		// Collect bullet points under this header
		i++
		for i < len(lines) {
			bullet := strings.TrimSpace(lines[i])
			if bullet == "" {
				i++
				continue
			}
			// Stop at next header or non-bullet content
			if strings.HasPrefix(bullet, "#") {
				break
			}
			if strings.HasPrefix(bullet, "- ") || strings.HasPrefix(bullet, "* ") || strings.HasPrefix(bullet, "• ") {
				text := strings.TrimLeft(bullet, "-*• ")
				if text != "" {
					insights = append(insights, Insight{
						Text:      text,
						Source:    source,
						TurnIndex: turnIdx,
					})
				}
			} else {
				// Not a bullet — stop collecting
				break
			}
			i++
		}
	}
	return insights
}

// extractSignalPhrases finds lines containing key signal phrases.
func extractSignalPhrases(lines []string, turnIdx int) []Insight {
	var insights []Insight
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ">") {
			continue
		}
		if !signalPhraseRe.MatchString(trimmed) {
			continue
		}
		// Skip if it's a bold-prefixed line (already caught by keyPhraseRe)
		if keyPhraseRe.MatchString(trimmed) {
			continue
		}
		// Use the whole line/sentence as the insight
		source := SourcePattern
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "decision") {
			source = SourceDecision
		}
		insights = append(insights, Insight{
			Text:      trimmed,
			Source:    source,
			TurnIndex: turnIdx,
		})
	}
	return insights
}
