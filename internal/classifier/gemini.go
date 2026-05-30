package classifier

func NewGemini(cfg OpenAIConfig) Classifier {
	if cfg.URL == "" {
		cfg.URL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-2.0-flash"
	}
	return NewOpenAI(cfg)
}
