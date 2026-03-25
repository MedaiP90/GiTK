// Package prefs implements the Preferences window using AdwPreferencesWindow.
//
// The preferences window follows GNOME HIG with preference groups for:
//   - Git (identity, fetch behavior, pruning)
//   - AI Settings (enable/disable, API key, model dropdown)
//
// Changes are saved to the config file when the window is closed.
package prefs

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/MedaiP90/GiTK/config"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Available Claude models for the AI dropdown.
var claudeModels = []string{
	"claude-sonnet-4-20250514",
	"claude-opus-4-20250514",
	"claude-haiku-4-5-20251001",
}

// Show creates and presents the preferences window.
//
// Parameters:
//   - parent: the parent window for transient-for relationship.
//   - cfg: the configuration to read/write.
func Show(parent *adw.ApplicationWindow, cfg *config.Config) {
	// AdwPreferencesWindow is the GNOME HIG way to show preferences.
	win := adw.NewPreferencesWindow()
	win.SetTitle("Preferences")
	win.SetTransientFor(&parent.Window)
	win.SetModal(true)

	// --- Git Page ---
	gitPage := adw.NewPreferencesPage()
	gitPage.SetTitle("Git")
	gitPage.SetIconName("vcs-branch-symbolic")

	// Identity subsection.
	identityGroup := adw.NewPreferencesGroup()
	identityGroup.SetTitle("Identity")
	identityGroup.SetDescription("Used as author when creating commits")

	nameRow := adw.NewEntryRow()
	nameRow.SetTitle("Author Name")
	nameRow.SetText(cfg.Git.AuthorName)
	identityGroup.Add(nameRow)

	emailRow := adw.NewEntryRow()
	emailRow.SetTitle("Author Email")
	emailRow.SetText(cfg.Git.AuthorEmail)
	identityGroup.Add(emailRow)

	perRepoRow := adw.NewSwitchRow()
	perRepoRow.SetTitle("Use Per-Repository Identity")
	perRepoRow.SetSubtitle("When enabled, uses the identity from .git/config instead")
	perRepoRow.SetActive(cfg.Git.PerRepoOverride)
	identityGroup.Add(perRepoRow)

	gitPage.Add(identityGroup)

	// Fetch subsection.
	fetchGroup := adw.NewPreferencesGroup()
	fetchGroup.SetTitle("Fetch")
	fetchGroup.SetDescription("Configure automatic remote fetching")

	autoFetchRow := adw.NewSwitchRow()
	autoFetchRow.SetTitle("Auto-Refresh")
	autoFetchRow.SetSubtitle("Periodically fetch from all remotes")
	autoFetchRow.SetActive(cfg.Git.AutoFetch)
	fetchGroup.Add(autoFetchRow)

	autoFetchIntervalRow := adw.NewEntryRow()
	autoFetchIntervalRow.SetTitle("Fetch Interval (minutes)")
	autoFetchIntervalRow.SetText(fmt.Sprintf("%d", cfg.Git.AutoFetchInterval))
	fetchGroup.Add(autoFetchIntervalRow)

	pruneRow := adw.NewSwitchRow()
	pruneRow.SetTitle("Prune When Fetching")
	pruneRow.SetSubtitle("Remove remote-tracking branches that no longer exist on the remote")
	pruneRow.SetActive(cfg.Git.PruneOnFetch)
	fetchGroup.Add(pruneRow)

	gitPage.Add(fetchGroup)
	win.Add(gitPage)

	// --- AI Settings Page ---
	aiPage := adw.NewPreferencesPage()
	aiPage.SetTitle("AI")
	aiPage.SetIconName("applications-science-symbolic")

	aiGroup := adw.NewPreferencesGroup()
	aiGroup.SetTitle("Claude AI Integration")
	aiGroup.SetDescription("Generate commit messages using Claude AI")

	aiEnabledRow := adw.NewSwitchRow()
	aiEnabledRow.SetTitle("Enable AI Features")
	aiEnabledRow.SetActive(cfg.AI.Enabled)
	aiGroup.Add(aiEnabledRow)

	// API key entry (password-style).
	apiKeyRow := adw.NewPasswordEntryRow()
	apiKeyRow.SetTitle("API Key")
	if cfg.AI.APIKey != "" {
		apiKeyRow.SetText(cfg.AI.APIKey)
	}
	aiGroup.Add(apiKeyRow)

	// Model selection as a combo row (dropdown).
	modelRow := adw.NewComboRow()
	modelRow.SetTitle("Model")
	modelRow.SetSubtitle("Select the Claude model for commit message generation")

	// Build a string list for the combo row.
	modelList := gtk.NewStringList(claudeModels)
	modelRow.SetModel(modelList)

	// Set the active model based on config.
	for i, m := range claudeModels {
		if m == cfg.AI.Model {
			modelRow.SetSelected(uint(i))
			break
		}
	}

	aiGroup.Add(modelRow)

	aiPage.Add(aiGroup)
	win.Add(aiPage)

	// Save settings when the window is closed.
	win.ConnectCloseRequest(func() bool {
		// Read values from UI and update config.
		cfg.Git.AuthorName = nameRow.Text()
		cfg.Git.AuthorEmail = emailRow.Text()
		cfg.Git.PerRepoOverride = perRepoRow.Active()
		cfg.Git.AutoFetch = autoFetchRow.Active()
		if interval, err := strconv.Atoi(autoFetchIntervalRow.Text()); err == nil && interval > 0 {
			cfg.Git.AutoFetchInterval = interval
		}
		cfg.Git.PruneOnFetch = pruneRow.Active()

		cfg.AI.Enabled = aiEnabledRow.Active()
		cfg.AI.APIKey = apiKeyRow.Text()

		// Read selected model from combo row.
		selectedIdx := modelRow.Selected()
		if int(selectedIdx) < len(claudeModels) {
			cfg.AI.Model = claudeModels[selectedIdx]
		}

		if err := cfg.Save(); err != nil {
			slog.Warn("failed to save preferences", "error", err)
		}

		return false // Allow the window to close.
	})

	win.Present()
}
