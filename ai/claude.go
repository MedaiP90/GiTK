// Package ai provides Claude AI integration for generating commit messages.
//
// The client sends the staged diff to the Claude API and receives a
// suggested commit message. The API key is read from the environment
// variable ANTHROPIC_API_KEY.
//
// This is an optional feature — it only activates when the user enables
// AI in preferences and provides an API key.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// DefaultModel is the Claude model used by default.
const DefaultModel = "claude-sonnet-4-20250514"

// APIEndpoint is the Claude Messages API endpoint.
const APIEndpoint = "https://api.anthropic.com/v1/messages"

// Client is the Claude AI client for commit message generation.
type Client struct {
	// apiKey is the Anthropic API key.
	apiKey string

	// model is the Claude model to use.
	model string

	// httpClient is the HTTP client with timeout.
	httpClient *http.Client
}

// NewClient creates a new Claude AI client.
// It uses the provided API key, falling back to the ANTHROPIC_API_KEY
// environment variable. Returns nil if no API key is available.
func NewClient(apiKey, model string) *Client {
	// Fall back to environment variable if no key provided via config.
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	if model == "" {
		model = DefaultModel
	}

	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// messagesRequest is the Claude Messages API request body.
type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}

// message is a single message in the conversation.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the Claude Messages API response body.
type messagesResponse struct {
	Content []contentBlock `json:"content"`
	Error   *apiError      `json:"error,omitempty"`
}

// contentBlock is a content block in the response.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// apiError is an error from the API.
type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// GenerateCommitMessage sends the staged diff to Claude and returns
// a suggested commit message.
//
// Parameters:
//   - ctx: context for cancellation.
//   - diff: the staged diff text.
//   - systemPrompt: optional custom system prompt (empty uses default).
func (c *Client) GenerateCommitMessage(ctx context.Context, diff string, systemPrompt string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("AI client not initialized (missing ANTHROPIC_API_KEY)")
	}

	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	// Build the request.
	reqBody := messagesRequest{
		Model:     c.model,
		MaxTokens: 512,
		System:    systemPrompt,
		Messages: []message{
			{
				Role:    "user",
				Content: BuildCommitMessagePrompt(diff),
			},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, "POST", APIEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response.
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	// Parse response.
	var respBody messagesResponse
	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if respBody.Error != nil {
		return "", fmt.Errorf("API error: %s", respBody.Error.Message)
	}

	if len(respBody.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return respBody.Content[0].Text, nil
}
