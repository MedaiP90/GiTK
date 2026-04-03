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

// Available OpenCode Go models (subscription plan, prefixed with opencode-go/).
var openCodeModels = []string{
	"opencode-go/kimi-k2.5",
	"opencode-go/glm-5",
	"opencode-go/minimax-m2.5",
}

// Available Google Gemini models.
var geminiModels = []string{
	"gemini-2.5-flash",
	"gemini-2.5-pro",
}

// aiProviders lists the display names shown in the provider combo row.
var aiProviders = []string{"Claude (Anthropic)", "OpenCode Go", "Google Gemini"}

// aiProviderKeys maps display-name index → config key.
var aiProviderKeys = []string{"claude", "opencode", "gemini"}

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

	// --- App Page ---
	appPage := adw.NewPreferencesPage()
	appPage.SetTitle("App")
	appPage.SetIconName("preferences-system-symbolic")

	// Refresh subsection.
	refreshGroup := adw.NewPreferencesGroup()
	refreshGroup.SetTitle("Refresh")
	refreshGroup.SetDescription("Configure automatic remote fetching")

	autoFetchRow := adw.NewSwitchRow()
	autoFetchRow.SetTitle("Auto-Refresh")
	autoFetchRow.SetSubtitle("Periodically fetch from all remotes")
	autoFetchRow.SetActive(cfg.Git.AutoFetch)
	refreshGroup.Add(autoFetchRow)

	autoFetchIntervalRow := adw.NewEntryRow()
	autoFetchIntervalRow.SetTitle("Fetch Interval (minutes)")
	autoFetchIntervalRow.SetText(fmt.Sprintf("%d", cfg.Git.AutoFetchInterval))
	refreshGroup.Add(autoFetchIntervalRow)

	appPage.Add(refreshGroup)

	// Recent repositories subsection.
	recentGroup := adw.NewPreferencesGroup()
	recentGroup.SetTitle("Recent Repositories")
	recentGroup.SetDescription("Configure recent repositories behavior")

	maxRecentRow := adw.NewEntryRow()
	maxRecentRow.SetTitle("Maximum Recent Repositories")
	maxRecentRow.SetText(fmt.Sprintf("%d", cfg.MaxRecent))
	recentGroup.Add(maxRecentRow)

	appPage.Add(recentGroup)
	win.Add(appPage)

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

	// Fetch subsection (only prune remains here).
	fetchGroup := adw.NewPreferencesGroup()
	fetchGroup.SetTitle("Fetch")

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
	aiGroup.SetTitle("AI Integration")
	aiGroup.SetDescription("Generate commit messages using AI")

	aiEnabledRow := adw.NewSwitchRow()
	aiEnabledRow.SetTitle("Enable AI Features")
	aiEnabledRow.SetActive(cfg.AI.Enabled)
	aiGroup.Add(aiEnabledRow)

	// Provider selection.
	providerRow := adw.NewComboRow()
	providerRow.SetTitle("Provider")
	providerRow.SetSubtitle("Select the AI provider")
	providerList := gtk.NewStringList(aiProviders)
	providerRow.SetModel(providerList)
	selectedProvider := 0
	for i, key := range aiProviderKeys {
		if key == cfg.AI.Provider {
			selectedProvider = i
			break
		}
	}
	providerRow.SetSelected(uint(selectedProvider))
	aiGroup.Add(providerRow)

	aiPage.Add(aiGroup)

	// Claude settings group.
	claudeGroup := adw.NewPreferencesGroup()
	claudeGroup.SetTitle("Claude (Anthropic)")

	apiKeyRow := adw.NewPasswordEntryRow()
	apiKeyRow.SetTitle("Anthropic API Key")
	if cfg.AI.APIKey != "" {
		apiKeyRow.SetText(cfg.AI.APIKey)
	}
	claudeGroup.Add(apiKeyRow)

	claudeModelRow := adw.NewComboRow()
	claudeModelRow.SetTitle("Model")
	claudeModelList := gtk.NewStringList(claudeModels)
	claudeModelRow.SetModel(claudeModelList)
	for i, m := range claudeModels {
		if m == cfg.AI.Model {
			claudeModelRow.SetSelected(uint(i))
			break
		}
	}
	claudeGroup.Add(claudeModelRow)
	aiPage.Add(claudeGroup)

	// OpenCode settings group.
	openCodeGroup := adw.NewPreferencesGroup()
	openCodeGroup.SetTitle("OpenCode Go")
	openCodeGroup.SetDescription("OpenCode Go subscription — opencode.ai/zen/go")

	openCodeKeyRow := adw.NewPasswordEntryRow()
	openCodeKeyRow.SetTitle("OpenCode API Key")
	if cfg.AI.OpenCodeAPIKey != "" {
		openCodeKeyRow.SetText(cfg.AI.OpenCodeAPIKey)
	}
	openCodeGroup.Add(openCodeKeyRow)

	openCodeModelRow := adw.NewComboRow()
	openCodeModelRow.SetTitle("Model")
	openCodeModelList := gtk.NewStringList(openCodeModels)
	openCodeModelRow.SetModel(openCodeModelList)
	selectedOCModel := 0
	for i, m := range openCodeModels {
		if m == cfg.AI.OpenCodeModel {
			selectedOCModel = i
			break
		}
	}
	openCodeModelRow.SetSelected(uint(selectedOCModel))
	openCodeGroup.Add(openCodeModelRow)
	aiPage.Add(openCodeGroup)

	// Google Gemini settings group.
	geminiGroup := adw.NewPreferencesGroup()
	geminiGroup.SetTitle("Google Gemini")
	geminiGroup.SetDescription("Google Gemini API — ai.google.dev")

	geminiKeyRow := adw.NewPasswordEntryRow()
	geminiKeyRow.SetTitle("Gemini API Key")
	if cfg.AI.GeminiAPIKey != "" {
		geminiKeyRow.SetText(cfg.AI.GeminiAPIKey)
	}
	geminiGroup.Add(geminiKeyRow)

	geminiModelRow := adw.NewComboRow()
	geminiModelRow.SetTitle("Model")
	geminiModelList := gtk.NewStringList(geminiModels)
	geminiModelRow.SetModel(geminiModelList)
	selectedGeminiModel := 0
	for i, m := range geminiModels {
		if m == cfg.AI.GeminiModel {
			selectedGeminiModel = i
			break
		}
	}
	geminiModelRow.SetSelected(uint(selectedGeminiModel))
	geminiGroup.Add(geminiModelRow)
	aiPage.Add(geminiGroup)

	win.Add(aiPage)

	// Save settings when the window is closed.
	win.ConnectCloseRequest(func() bool {
		// Read values from UI and update config.
		// App settings.
		cfg.Git.AutoFetch = autoFetchRow.Active()
		if interval, err := strconv.Atoi(autoFetchIntervalRow.Text()); err == nil && interval > 0 {
			cfg.Git.AutoFetchInterval = interval
		}
		if maxRecent, err := strconv.Atoi(maxRecentRow.Text()); err == nil && maxRecent > 0 {
			cfg.MaxRecent = maxRecent
		}

		// Git settings.
		cfg.Git.AuthorName = nameRow.Text()
		cfg.Git.AuthorEmail = emailRow.Text()
		cfg.Git.PerRepoOverride = perRepoRow.Active()
		cfg.Git.PruneOnFetch = pruneRow.Active()

		cfg.AI.Enabled = aiEnabledRow.Active()

		// Provider.
		pIdx := providerRow.Selected()
		if int(pIdx) < len(aiProviderKeys) {
			cfg.AI.Provider = aiProviderKeys[pIdx]
		}

		// Claude settings.
		cfg.AI.APIKey = apiKeyRow.Text()
		claudeIdx := claudeModelRow.Selected()
		if int(claudeIdx) < len(claudeModels) {
			cfg.AI.Model = claudeModels[claudeIdx]
		}

		// OpenCode settings.
		cfg.AI.OpenCodeAPIKey = openCodeKeyRow.Text()
		ocIdx := openCodeModelRow.Selected()
		if int(ocIdx) < len(openCodeModels) {
			cfg.AI.OpenCodeModel = openCodeModels[ocIdx]
		}

		// Gemini settings.
		cfg.AI.GeminiAPIKey = geminiKeyRow.Text()
		geminiIdx := geminiModelRow.Selected()
		if int(geminiIdx) < len(geminiModels) {
			cfg.AI.GeminiModel = geminiModels[geminiIdx]
		}

		if err := cfg.Save(); err != nil {
			slog.Warn("failed to save preferences", "error", err)
		}

		return false // Allow the window to close.
	})

	win.Present()
}
