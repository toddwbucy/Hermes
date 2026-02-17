package version

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// UpdateAvailableMsg is sent when a new sidecar version is available.
type UpdateAvailableMsg struct {
	CurrentVersion string
	LatestVersion  string
	UpdateCommand  string
	ReleaseNotes   string
	ReleaseURL     string
	InstallMethod  InstallMethod
}

// TdVersionMsg is sent with td version info (installed or not).
type TdVersionMsg struct {
	Installed      bool
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
}

// updateCommand generates the update command based on install method.
func updateCommand(version string, method InstallMethod) string {
	switch method {
	case InstallMethodHomebrew:
		return "brew upgrade hermes"
	case InstallMethodBinary:
		return fmt.Sprintf("https://github.com/toddwbucy/hermes/releases/tag/%s", version)
	default:
		return fmt.Sprintf(
			"go install -ldflags \"-X main.Version=%s\" github.com/toddwbucy/hermes/cmd/hermes@%s",
			version, version,
		)
	}
}

// CheckAsync returns a Bubble Tea command that checks for updates in background.
func CheckAsync(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		method := DetectInstallMethod()

		// Check cache first
		if cached, err := LoadCache(); err == nil && IsCacheValid(cached, currentVersion) {
			if cached.HasUpdate {
				return UpdateAvailableMsg{
					CurrentVersion: currentVersion,
					LatestVersion:  cached.LatestVersion,
					UpdateCommand:  updateCommand(cached.LatestVersion, method),
					InstallMethod:  method,
				}
			}
			return nil // up-to-date, cached
		}

		// Cache miss or invalid, fetch from GitHub
		result := Check(currentVersion)

		// Only cache successful checks (don't cache network errors)
		if result.Error == nil {
			_ = SaveCache(&CacheEntry{
				LatestVersion:  result.LatestVersion,
				CurrentVersion: currentVersion,
				CheckedAt:      time.Now(),
				HasUpdate:      result.HasUpdate,
			})
		}

		if result.HasUpdate {
			return UpdateAvailableMsg{
				CurrentVersion: currentVersion,
				LatestVersion:  result.LatestVersion,
				UpdateCommand:  updateCommand(result.LatestVersion, method),
				ReleaseNotes:   result.ReleaseNotes,
				ReleaseURL:     result.UpdateURL,
				InstallMethod:  method,
			}
		}

		return nil
	}
}

// ForceCheckAsync checks for updates, ignoring the cache.
func ForceCheckAsync(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		method := DetectInstallMethod()
		result := Check(currentVersion)
		if result.Error == nil {
			_ = SaveCache(&CacheEntry{
				LatestVersion:  result.LatestVersion,
				CurrentVersion: currentVersion,
				CheckedAt:      time.Now(),
				HasUpdate:      result.HasUpdate,
			})
		}
		if result.HasUpdate {
			return UpdateAvailableMsg{
				CurrentVersion: currentVersion,
				LatestVersion:  result.LatestVersion,
				UpdateCommand:  updateCommand(result.LatestVersion, method),
				ReleaseNotes:   result.ReleaseNotes,
				ReleaseURL:     result.UpdateURL,
				InstallMethod:  method,
			}
		}
		return nil
	}
}

// CheckTdAsync returns not-installed (td integration removed).
func CheckTdAsync() tea.Cmd {
	return func() tea.Msg {
		return TdVersionMsg{Installed: false}
	}
}

// ForceCheckTdAsync returns not-installed (td integration removed).
func ForceCheckTdAsync() tea.Cmd {
	return func() tea.Msg {
		return TdVersionMsg{Installed: false}
	}
}
