// Package prefs implements the Preferences window using AdwPreferencesWindow.
//
// The preferences window follows GNOME HIG with preference groups for:
//   - Git Identity (name, email)
//   - Graph Settings (max commits, show tags, show remotes)
//   - AI Settings (enable/disable, model selection)
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

// Show creates and presents the preferences window.
//
// Parameters:
//   - parent: the parent window for transient-for relationship.
//   - cfg: the configuration to read/write.
func Show(parent *adw.ApplicationWindow, cfg *config.Config) {
	// AdwPreferencesWindow is the GNOME HIG way to show preferences.
	// It provides built-in search and navigation between preference pages.
	win := adw.NewPreferencesWindow()
	win.SetTitle("Preferences")
	win.SetTransientFor(&parent.Window)
	win.SetModal(true)

	// --- Git Identity Page ---
	identityPage := adw.NewPreferencesPage()
	identityPage.SetTitle("Identity")
	identityPage.SetIconName("avatar-default-symbolic")

	identityGroup := adw.NewPreferencesGroup()
	identityGroup.SetTitle("Git Identity")
	identityGroup.SetDescription("Used as author when creating commits")

	// Author name.
	nameRow := adw.NewEntryRow()
	nameRow.SetTitle("Author Name")
	nameRow.SetText(cfg.GitIdentity.Name)
	identityGroup.Add(nameRow)

	// Author email.
	emailRow := adw.NewEntryRow()
	emailRow.SetTitle("Author Email")
	emailRow.SetText(cfg.GitIdentity.Email)
	identityGroup.Add(emailRow)

	// Per-repo override switch.
	perRepoRow := adw.NewSwitchRow()
	perRepoRow.SetTitle("Use Per-Repository Identity")
	perRepoRow.SetSubtitle("When enabled, uses the identity from .git/config instead")
	perRepoRow.SetActive(cfg.GitIdentity.PerRepoOverride)
	identityGroup.Add(perRepoRow)

	identityPage.Add(identityGroup)
	win.Add(identityPage)

	// --- Graph Settings Page ---
	graphPage := adw.NewPreferencesPage()
	graphPage.SetTitle("Graph")
	graphPage.SetIconName("view-list-symbolic")

	graphGroup := adw.NewPreferencesGroup()
	graphGroup.SetTitle("Graph View Settings")

	// Max commits.
	maxCommitsRow := adw.NewEntryRow()
	maxCommitsRow.SetTitle("Maximum Commits")
	maxCommitsRow.SetText(fmt.Sprintf("%d", cfg.Graph.MaxCommits))
	graphGroup.Add(maxCommitsRow)

	// Show tags toggle.
	showTagsRow := adw.NewSwitchRow()
	showTagsRow.SetTitle("Show Tags")
	showTagsRow.SetActive(cfg.Graph.ShowTags)
	graphGroup.Add(showTagsRow)

	// Show remotes toggle.
	showRemotesRow := adw.NewSwitchRow()
	showRemotesRow.SetTitle("Show Remote Branches")
	showRemotesRow.SetActive(cfg.Graph.ShowRemotes)
	graphGroup.Add(showRemotesRow)

	graphPage.Add(graphGroup)
	win.Add(graphPage)

	// --- AI Settings Page ---
	aiPage := adw.NewPreferencesPage()
	aiPage.SetTitle("AI")
	aiPage.SetIconName("applications-science-symbolic")

	aiGroup := adw.NewPreferencesGroup()
	aiGroup.SetTitle("Claude AI Integration")
	aiGroup.SetDescription("Generate commit messages using Claude AI")

	// AI enabled toggle.
	aiEnabledRow := adw.NewSwitchRow()
	aiEnabledRow.SetTitle("Enable AI Features")
	aiEnabledRow.SetActive(cfg.AI.Enabled)
	aiGroup.Add(aiEnabledRow)

	// Model selection.
	modelRow := adw.NewEntryRow()
	modelRow.SetTitle("Model")
	modelRow.SetText(cfg.AI.Model)
	aiGroup.Add(modelRow)

	aiPage.Add(aiGroup)
	win.Add(aiPage)

	// Save settings when the window is closed.
	win.ConnectCloseRequest(func() bool {
		// Read values from UI and update config.
		cfg.GitIdentity.Name = nameRow.Text()
		cfg.GitIdentity.Email = emailRow.Text()
		cfg.GitIdentity.PerRepoOverride = perRepoRow.Active()

		if maxCommits, err := strconv.Atoi(maxCommitsRow.Text()); err == nil && maxCommits > 0 {
			cfg.Graph.MaxCommits = maxCommits
		}
		cfg.Graph.ShowTags = showTagsRow.Active()
		cfg.Graph.ShowRemotes = showRemotesRow.Active()

		cfg.AI.Enabled = aiEnabledRow.Active()
		cfg.AI.Model = modelRow.Text()

		if err := cfg.Save(); err != nil {
			slog.Warn("failed to save preferences", "error", err)
		}

		return false // Allow the window to close.
	})

	win.Present()

	// Suppress unused import warning for gtk.
	_ = gtk.NewBox
}
