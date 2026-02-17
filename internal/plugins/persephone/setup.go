package persephone

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toddwbucy/hermes/internal/plugin"
	"github.com/toddwbucy/hermes/internal/styles"
)

// setupModel handles the first-run setup wizard.
type setupModel struct {
	workDir  string
	input    string
	cursor   int
	errMsg   string
	repoName string
}

func newSetupModel(workDir string) *setupModel {
	// Auto-detect repo name for suggested database name
	repoName := detectRepoName(workDir)
	return &setupModel{
		workDir:  workDir,
		input:    repoName, // Pre-fill with repo name as suggestion
		cursor:   len(repoName),
		repoName: repoName,
	}
}

// handleKey processes key events for the setup wizard.
func (s *setupModel) handleKey(p *Plugin, msg tea.KeyMsg) (plugin.Plugin, tea.Cmd) {
	switch msg.String() {
	case "enter":
		db := strings.TrimSpace(s.input)
		if db == "" {
			s.errMsg = "database name is required"
			return p, nil
		}
		// Write config and finish setup
		if err := s.writeConfig(db); err != nil {
			s.errMsg = fmt.Sprintf("write config: %v", err)
			return p, nil
		}
		s.ensureGitignore()
		return p, func() tea.Msg { return SetupCompleteMsg{Database: db} }

	case "backspace":
		if s.cursor > 0 && len(s.input) > 0 {
			s.input = s.input[:s.cursor-1] + s.input[s.cursor:]
			s.cursor--
		}

	case "left":
		if s.cursor > 0 {
			s.cursor--
		}
	case "right":
		if s.cursor < len(s.input) {
			s.cursor++
		}

	case "esc":
		// Can't escape setup — need a database to function

	default:
		// Type character
		if len(msg.String()) == 1 {
			ch := msg.String()
			s.input = s.input[:s.cursor] + ch + s.input[s.cursor:]
			s.cursor++
			s.errMsg = ""
		}
	}

	return p, nil
}

// ConsumesTextInput implements plugin.TextInputConsumer.
func (s *setupModel) consumesTextInput() bool { return true }

func (s *setupModel) view(width, height int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	labelStyle := lipgloss.NewStyle().Foreground(styles.TextSecondary)
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.BorderActive).
		Padding(0, 1).
		Width(40)

	var lines []string

	lines = append(lines, titleStyle.Render("Persephone Setup"))
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("No .hermes/config.yaml found for this workspace."))
	lines = append(lines, labelStyle.Render("Which HADES database should this project use?"))
	lines = append(lines, "")

	// Render input with cursor
	displayInput := s.input
	if s.cursor >= 0 && s.cursor <= len(displayInput) {
		before := displayInput[:s.cursor]
		after := ""
		if s.cursor < len(displayInput) {
			after = displayInput[s.cursor:]
		}
		displayInput = before + "█" + after
	}
	lines = append(lines, inputStyle.Render(displayInput))

	if s.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(styles.Error)
		lines = append(lines, errStyle.Render(s.errMsg))
	}

	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Press Enter to confirm"))

	content := strings.Join(lines, "\n")

	// Center in available space
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// writeConfig creates .hermes/config.yaml with the database name.
func (s *setupModel) writeConfig(database string) error {
	dir := filepath.Join(s.workDir, ".hermes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("# Hermes workspace config\ndatabase: %s\n", database)
	return os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644)
}

// ensureGitignore adds .hermes/ to .gitignore if not already present.
func (s *setupModel) ensureGitignore() {
	gitignorePath := filepath.Join(s.workDir, ".gitignore")

	var existing string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	// Check if .hermes/ already in gitignore
	for _, line := range strings.Split(existing, "\n") {
		if strings.TrimSpace(line) == ".hermes/" {
			return
		}
	}

	// Append
	var toAppend string
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		toAppend = "\n"
	}
	toAppend += "\n# Hermes workspace config\n.hermes/\n"

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(toAppend)
}

// detectRepoName extracts the repository name from git remote or directory name.
func detectRepoName(workDir string) string {
	// Try git remote
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = workDir
	if output, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(output))
		// Extract repo name from URL
		url = strings.TrimSuffix(url, ".git")
		if idx := strings.LastIndex(url, "/"); idx >= 0 {
			return url[idx+1:]
		}
	}

	// Fallback to directory name
	return filepath.Base(workDir)
}
