package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ClaudeConfig struct {
	APIKey string
	Model  string
	URL    string
}

type claudeProvider struct {
	config ClaudeConfig
	client *http.Client
}

func NewClaude(cfg ClaudeConfig) Classifier {
	url := strings.TrimRight(cfg.URL, "/")
	if url == "" {
		url = "https://api.anthropic.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &claudeProvider{
		config: ClaudeConfig{
			APIKey: cfg.APIKey,
			Model:  model,
			URL:    url,
		},
		client: &http.Client{},
	}
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeResponse struct {
	Content []claudeContentBlock `json:"content"`
}

func (c *claudeProvider) Classify(ctx context.Context, input ClassifyInput) (Category, error) {
	categories := []string{"needs_response", "personal", "marketing", "newsletter", "transactional", "notification", "spam"}

	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(input.From)
	b.WriteString("\nTo: ")
	b.WriteString(input.To)
	b.WriteString("\nSubject: ")
	b.WriteString(input.Subject)
	b.WriteString("\nBody:\n")
	if len(input.Body) > 4096 {
		b.WriteString(input.Body[:4096])
	} else {
		b.WriteString(input.Body)
	}

	body := claudeRequest{
		Model:     c.config.Model,
		MaxTokens: 20,
		System: fmt.Sprintf(
			`You are an email classifier. Classify the email into exactly one of these categories: %s. Respond with ONLY the category name, nothing else.`,
			strings.Join(categories, ", "),
		),
		Messages: []claudeMessage{
			{Role: "user", Content: b.String()},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("classifier: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("classifier: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("classifier: api request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("classifier: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("classifier: api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return "", fmt.Errorf("classifier: parse response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("classifier: no content in response")
	}

	category := strings.TrimSpace(claudeResp.Content[0].Text)
	for _, c := range categories {
		if strings.EqualFold(category, c) {
			return Category(c), nil
		}
	}

	return "", fmt.Errorf("classifier: unrecognized category: %s", category)
}
