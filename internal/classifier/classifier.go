package classifier

import (
	"context"
	"fmt"

	"github.com/colormechadd/mailaroo/internal/config"
)

type Category string

const (
	NeedsResponse Category = "needs_response"
	Personal      Category = "personal"
	Marketing     Category = "marketing"
	Newsletter    Category = "newsletter"
	Transactional Category = "transactional"
	Notification  Category = "notification"
	Spam          Category = "spam"
)

type ClassifyInput struct {
	From    string
	To      string
	Subject string
	Body    string
}

type Classifier interface {
	Classify(ctx context.Context, input ClassifyInput) (Category, error)
}

func New(cfg config.ClassifierConfig) (Classifier, error) {
	switch cfg.Type {
	case "openai":
		return NewOpenAI(OpenAIConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		}), nil
	case "deepseek":
		return NewDeepSeek(OpenAIConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		}), nil
	case "gemini":
		return NewGemini(OpenAIConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		}), nil
	case "claude":
		return NewClaude(ClaudeConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		}), nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("classifier: unknown type %q", cfg.Type)
	}
}
