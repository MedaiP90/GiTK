// Package ai — opencode.go implements the OpenCode Go AI provider.
//
// OpenCode Go exposes an OpenAI-compatible chat completions endpoint at
// https://opencode.ai/zen/go/v1/chat/completions.
// API keys are obtained from the OpenCode Zen subscription.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenCodeAPIEndpoint is the OpenCode Go chat completions endpoint.
const OpenCodeAPIEndpoint = "https://opencode.ai/zen/go/v1/chat/completions"

// DefaultOpenCodeModel is the default model for the OpenCode Go API.
const DefaultOpenCodeModel = "claude-sonnet-4-5"

// Available OpenCode Go models.
var OpenCodeModels = []string{
	"claude-sonnet-4-5",
	"claude-haiku-4-5",
	"gpt-4o",
	"gpt-4o-mini",
}

// OpenCodeClient is the AI client for the OpenCode Go subscription.
type OpenCodeClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenCodeClient creates a new OpenCode AI client.
// Returns nil if no API key is provided.
func NewOpenCodeClient(apiKey, model string) *OpenCodeClient {
	if apiKey == "" {
		return nil
	}
	if model == "" {
		model = DefaultOpenCodeModel
	}
	return &OpenCodeClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// openAIChatRequest is the OpenAI-compatible request body.
type openAIChatRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	Messages  []openAIChatMsg   `json:"messages"`
}

// openAIChatMsg is a single message in the OpenAI chat format.
type openAIChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the OpenAI-compatible response body.
type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
	Error   *openAIError   `json:"error,omitempty"`
}

// openAIChoice is one completion choice in the response.
type openAIChoice struct {
	Message openAIChatMsg `json:"message"`
}

// openAIError is an error object returned by OpenAI-compatible APIs.
type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// GenerateCommitMessage sends the staged diff to the OpenCode Go API and
// returns a suggested commit message.
func (c *OpenCodeClient) GenerateCommitMessage(ctx context.Context, diff string, systemPrompt string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("OpenCode client not initialised (missing API key)")
	}

	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	reqBody := openAIChatRequest{
		Model:     c.model,
		MaxTokens: 512,
		Messages: []openAIChatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: BuildCommitMessagePrompt(diff)},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", OpenCodeAPIEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var respBody openAIChatResponse
	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if respBody.Error != nil {
		return "", fmt.Errorf("API error: %s", respBody.Error.Message)
	}

	if len(respBody.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return respBody.Choices[0].Message.Content, nil
}
