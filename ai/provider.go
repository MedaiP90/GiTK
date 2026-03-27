// Package ai — provider.go defines the AIProvider interface and a factory
// function that creates the appropriate implementation based on config.
package ai

import (
	"context"
	"os"

	"github.com/MedaiP90/GiTK/config"
)

// ProviderName constants for the supported AI providers.
const (
	ProviderClaude   = "claude"
	ProviderOpenCode = "opencode"
)

// AIProvider is the common interface for AI backends that can generate
// commit messages.
type AIProvider interface {
	// GenerateCommitMessage sends the staged diff to the AI provider and
	// returns a suggested commit message.
	GenerateCommitMessage(ctx context.Context, diff string, systemPrompt string) (string, error)
}

// NewProvider creates the appropriate AIProvider based on cfg.
// Returns nil if the provider cannot be initialised (e.g., missing API key).
func NewProvider(cfg config.AIConfig) AIProvider {
	switch cfg.Provider {
	case ProviderOpenCode:
		apiKey := cfg.OpenCodeAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENCODE_API_KEY")
		}
		model := cfg.OpenCodeModel
		return NewOpenCodeClient(apiKey, model)
	default:
		// Default to Claude.
		return NewClient(cfg.APIKey, cfg.Model)
	}
}
