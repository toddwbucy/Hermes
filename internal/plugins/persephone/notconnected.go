package persephone

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwbucy/hermes/internal/styles"
)

// renderNotConnected shows a helpful message when ArangoDB is unreachable.
func renderNotConnected(width, height int, database, errMsg string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Error)
	labelStyle := lipgloss.NewStyle().Foreground(styles.TextSecondary)
	codeStyle := lipgloss.NewStyle().Foreground(styles.TextMuted)

	var lines []string

	lines = append(lines, titleStyle.Render("Persephone — Not Connected"))
	lines = append(lines, "")

	if database != "" {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("Database: %s", database)))
	} else {
		lines = append(lines, labelStyle.Render("No database configured"))
	}

	if errMsg != "" {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("Error: %s", errMsg)))
	}

	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Troubleshooting:"))
	lines = append(lines, codeStyle.Render("  1. Check ArangoDB is running:"))
	lines = append(lines, codeStyle.Render("     systemctl status arangodb3"))
	lines = append(lines, codeStyle.Render("  2. Verify database exists:"))
	lines = append(lines, codeStyle.Render(fmt.Sprintf("     HADES_DATABASE=%s hades db collections", database)))
	lines = append(lines, codeStyle.Render("  3. Check ARANGO_PASSWORD is set:"))
	lines = append(lines, codeStyle.Render("     echo $ARANGO_PASSWORD"))
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Config location: .hermes/config.yaml"))
	lines = append(lines, labelStyle.Render("Override with: HADES_DATABASE=<name> hermes"))

	content := strings.Join(lines, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
