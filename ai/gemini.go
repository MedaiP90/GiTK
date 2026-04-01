// Package ai — gemini.go implements the AIProvider interface for Google Gemini.
//
// The Gemini REST API is called directly using net/http + encoding/json.
// No external SDK is required.
//
// API reference:
//
//	POST https://generativelanguage.googleapis.com/v1beta/models/<model>:generateContent?key=<apiKey>
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	geminiAPIBase    = "https://generativelanguage.googleapis.com/v1beta/models"
	geminiDefaultModel = "gemini-2.5-flash"
)

// GeminiClient implements AIProvider using the Google Gemini REST API.
type GeminiClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewGeminiClient creates a new GeminiClient.
func NewGeminiClient(apiKey, model string) *GeminiClient {
	if model == "" {
		model = geminiDefaultModel
	}
	return &GeminiClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

// geminiRequest is the request body for the Gemini generateContent API.
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiResponse is the response body from the Gemini generateContent API.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GenerateCommitMessage sends the diff to Gemini and returns a commit message.
func (g *GeminiClient) GenerateCommitMessage(ctx context.Context, diff string, systemPrompt string) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("gemini: API key not configured")
	}

	prompt := systemPrompt
	if prompt == "" {
		prompt = "You are a helpful assistant that generates concise git commit messages. " +
			"Given the following diff, write a short, imperative commit message (max 72 chars for the subject). " +
			"Output only the commit message, no explanation."
	}
	prompt += "\n\nDiff:\n" + diff

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIBase, g.model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gemini: read response: %w", err)
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(respBytes, &gemResp); err != nil {
		return "", fmt.Errorf("gemini: parse response: %w", err)
	}

	if gemResp.Error != nil {
		return "", fmt.Errorf("gemini API error: %s", gemResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini: unexpected status %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: no content in response")
	}

	return gemResp.Candidates[0].Content.Parts[0].Text, nil
}
