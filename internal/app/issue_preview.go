package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// IssueSearchResult holds a single search result.
type IssueSearchResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Type     string `json:"type"`
	Priority string `json:"priority"`
}

// IssueSearchResultMsg carries search results back to the app.
type IssueSearchResultMsg struct {
	Query   string
	Results []IssueSearchResult
	Error   error
}

// issueSearchCmd returns empty results (td integration removed).
// TODO: Replace with Persephone task search via ArangoDB.
func issueSearchCmd(workDir, query string, includeClosed bool) tea.Cmd {
	return func() tea.Msg {
		return IssueSearchResultMsg{Query: query, Results: nil}
	}
}

// IssuePreviewData holds lightweight issue data.
type IssuePreviewData struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Type        string   `json:"type"`
	Priority    string   `json:"priority"`
	Points      int      `json:"points"`
	Description string   `json:"description"`
	ParentID    string   `json:"parent_id"`
	Labels      []string `json:"labels"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// IssuePreviewResultMsg carries fetched issue data back to the app.
type IssuePreviewResultMsg struct {
	Data  *IssuePreviewData
	Error error
}

// OpenFullIssueMsg is broadcast to plugins to open the full issue view.
type OpenFullIssueMsg struct {
	IssueID string
}

// fetchIssuePreviewCmd returns an error (td integration removed).
// TODO: Replace with Persephone task fetch via ArangoDB.
func fetchIssuePreviewCmd(workDir, issueID string) tea.Cmd {
	return func() tea.Msg {
		return IssuePreviewResultMsg{Error: fmt.Errorf("issue preview not available (td removed)")}
	}
}
