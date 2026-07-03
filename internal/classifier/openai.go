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

type OpenAIConfig struct {
	APIKey          string
	Model           string
	URL             string
	DisableThinking bool
}

type openAIProvider struct {
	config OpenAIConfig
	client *http.Client
}

func NewOpenAI(cfg OpenAIConfig) Classifier {
	url := strings.TrimRight(cfg.URL, "/")
	if url == "" {
		url = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &openAIProvider{
		config: OpenAIConfig{
			APIKey: cfg.APIKey,
			Model:  model,
			URL:    url,
		},
		client: &http.Client{},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []chatMessage    `json:"messages"`
	Temperature float64          `json:"temperature"`
	MaxTokens   int              `json:"max_tokens"`
	Thinking    *json.RawMessage `json:"thinking,omitempty"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

func (o *openAIProvider) Classify(ctx context.Context, input ClassifyInput) (Category, error) {
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

	body := chatRequest{
		Model: o.config.Model,
		Messages: []chatMessage{
			{
				Role: "system",
				Content: fmt.Sprintf(
					`You are an email classifier. Classify the email into exactly one of these categories: %s. Respond with ONLY the category name, nothing else.`,
					strings.Join(categories, ", "),
				),
			},
			{
				Role:    "user",
				Content: b.String(),
			},
		},
		Temperature: 0.1,
		MaxTokens:   20,
	}

	if o.config.DisableThinking {
		v := json.RawMessage(`{"type":"disabled"}`)
		body.Thinking = &v
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("classifier: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.config.URL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("classifier: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.config.APIKey)

	resp, err := o.client.Do(req)
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

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("classifier: parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("classifier: no choices in response")
	}

	category := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	for _, c := range categories {
		if strings.EqualFold(category, c) {
			return Category(c), nil
		}
	}

	return "", fmt.Errorf("classifier: unrecognized category: %s", category)
}
