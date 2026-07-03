package classifier

func NewDeepSeek(cfg OpenAIConfig) Classifier {
	if cfg.URL == "" {
		cfg.URL = "https://api.deepseek.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	cfg.DisableThinking = true
	return NewOpenAI(cfg)
}
