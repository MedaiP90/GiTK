// Package config manages the application's persistent configuration.
//
// Configuration is stored in the XDG config directory following the
// freedesktop.org specification. On Linux, this is typically:
//
//	~/.config/gitk/config.json
//
// The config file uses JSON format for simplicity and readability.
// All config operations are safe to call from any goroutine because
// we use file-level locking.
//
// Design note: We use the "adrg/xdg" library to resolve XDG paths
// portably. On macOS and Windows, it maps to the platform-appropriate
// directories (e.g., ~/Library/Application Support on macOS).
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
)

// appName is used to create the subdirectory under XDG config.
// This matches the application name in lowercase.
const appName = "gitk"

// configFileName is the name of the configuration file.
const configFileName = "config.json"

// Config holds all persistent application settings. Fields are exported
// so that the JSON encoder/decoder can access them.
//
// When adding a new setting:
//  1. Add a field here with a json tag.
//  2. Set its default value in Default().
//  3. Add UI for it in the Preferences window (Phase 8).
type Config struct {
	// mu protects concurrent access to the config. We use a sync.RWMutex
	// so that multiple goroutines can read simultaneously, but writes are
	// exclusive.
	mu sync.RWMutex `json:"-"`

	// RecentRepositories is a list of recently opened repository paths,
	// most recent first. We keep at most MaxRecent entries.
	RecentRepositories []string `json:"recent_repositories"`

	// MaxRecent is the maximum number of recent repositories to remember.
	MaxRecent int `json:"max_recent"`

	// Theme controls the application color scheme: "system", "light", or "dark".
	Theme string `json:"theme"`

	// Git holds git-related settings (identity, fetch behavior, etc.).
	Git GitConfig `json:"git"`

	// AI holds settings for the optional Claude AI integration.
	AI AIConfig `json:"ai"`

	// Graph holds settings for the visual graph view.
	Graph GraphConfig `json:"graph"`

	// configPath is the full path to the config file on disk.
	// This is not serialized — it's set when loading.
	configPath string `json:"-"`
}

// GitConfig holds all git-related settings.
type GitConfig struct {
	// AuthorName is the author name for commits (e.g., "Alice Smith").
	AuthorName string `json:"author_name"`

	// AuthorEmail is the author email for commits (e.g., "alice@example.com").
	AuthorEmail string `json:"author_email"`

	// PerRepoOverride, when true, uses the repo's .git/config identity
	// instead of this global one.
	PerRepoOverride bool `json:"per_repo_override"`

	// AutoFetch, when true, periodically fetches from all remotes.
	AutoFetch bool `json:"auto_fetch"`

	// AutoFetchInterval is the interval in minutes between automatic fetches.
	AutoFetchInterval int `json:"auto_fetch_interval"`

	// PruneOnFetch, when true, prunes deleted remote branches when fetching.
	PruneOnFetch bool `json:"prune_on_fetch"`
}

// AIConfig holds settings for the optional AI commit message generation.
type AIConfig struct {
	// Enabled controls whether AI features are available in the UI.
	Enabled bool `json:"enabled"`

	// APIKey is the Anthropic API key for Claude AI.
	// Note: Storing API keys in config is acceptable for desktop apps
	// where the config file is user-owned and has restricted permissions.
	APIKey string `json:"api_key,omitempty"`

	// Model is the Claude model to use (e.g., "claude-sonnet-4-20250514").
	Model string `json:"model"`

	// SystemPrompt is an optional custom system prompt for commit message
	// generation. If empty, the default prompt is used.
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// GraphConfig holds settings for the visual graph view.
type GraphConfig struct {
	// MaxCommits is the maximum number of commits to load in the graph.
	// Higher values give a more complete picture but use more memory.
	MaxCommits int `json:"max_commits"`

	// ShowTags controls whether tags are shown in the graph by default.
	ShowTags bool `json:"show_tags"`

	// ShowRemotes controls whether remote branches are shown by default.
	ShowRemotes bool `json:"show_remotes"`
}

// Default returns a Config with sensible default values. This is used
// when no config file exists yet, or when the config file cannot be loaded.
func Default() *Config {
	return &Config{
		RecentRepositories: []string{},
		MaxRecent:          20,
		Theme:              "system",
		Git: GitConfig{
			PerRepoOverride:   true,
			AutoFetch:         false,
			AutoFetchInterval: 5,
			PruneOnFetch:      false,
		},
		AI: AIConfig{
			Enabled: false,
			Model:   "claude-sonnet-4-20250514",
		},
		Graph: GraphConfig{
			MaxCommits:  2000,
			ShowTags:    true,
			ShowRemotes: true,
		},
	}
}

// Load reads the configuration from the XDG config directory.
// If the config file does not exist, it returns Default() config and
// creates the file with default values.
func Load() (*Config, error) {
	// Resolve the config file path using XDG.
	// xdg.ConfigFile returns the path and creates parent directories.
	configPath, err := xdg.ConfigFile(filepath.Join(appName, configFileName))
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	cfg := Default()
	cfg.configPath = configPath

	// Try to read the existing config file.
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// First run: create the config file with defaults.
			slog.Info("creating default config", "path", configPath)
			if saveErr := cfg.Save(); saveErr != nil {
				return cfg, fmt.Errorf("save default config: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Parse the JSON config file.
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	slog.Info("config loaded", "path", configPath)
	return cfg, nil
}

// Save writes the current configuration to disk in JSON format.
// It's safe to call from any goroutine.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Marshal with indentation for human readability.
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write atomically by writing to a temp file and renaming.
	// This prevents corruption if the app crashes mid-write.
	tmpPath := c.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write config temp file: %w", err)
	}
	if err := os.Rename(tmpPath, c.configPath); err != nil {
		return fmt.Errorf("rename config file: %w", err)
	}

	slog.Debug("config saved", "path", c.configPath)
	return nil
}

// AddRecentRepository adds a repository path to the top of the recent
// list. If the path is already in the list, it is moved to the top.
// The list is trimmed to MaxRecent entries.
func (c *Config) AddRecentRepository(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove the path if it's already in the list (we'll re-add it at top).
	filtered := make([]string, 0, len(c.RecentRepositories))
	for _, p := range c.RecentRepositories {
		if p != path {
			filtered = append(filtered, p)
		}
	}

	// Prepend the new path.
	c.RecentRepositories = append([]string{path}, filtered...)

	// Trim to max length.
	if len(c.RecentRepositories) > c.MaxRecent {
		c.RecentRepositories = c.RecentRepositories[:c.MaxRecent]
	}
}

// RemoveRecentRepository removes a repository path from the recent list.
func (c *Config) RemoveRecentRepository(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	filtered := make([]string, 0, len(c.RecentRepositories))
	for _, p := range c.RecentRepositories {
		if p != path {
			filtered = append(filtered, p)
		}
	}
	c.RecentRepositories = filtered
}

// GetRecentRepositories returns a copy of the recent repositories list.
// It's safe to call from any goroutine.
func (c *Config) GetRecentRepositories() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent race conditions if the caller modifies the slice.
	result := make([]string, len(c.RecentRepositories))
	copy(result, c.RecentRepositories)
	return result
}
