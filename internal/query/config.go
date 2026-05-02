package query

type Config struct {
	MaxTokens              int
	MaxToolIterations      int
	DebugTools             bool
	EvidenceToolExposure   bool
	PersistenceEnabled     bool
	MaxForcedContinuations int
	ResearchMinSearches    int
	ResearchMinFetches     int
	MaxSameFailureRetries  int
	// ContextWindowFallback — activeModelContextWindow()가 0을 반환할 때 사용할 기본값
	ContextWindowFallback int
}

func DefaultConfig() Config {
	return Config{
		MaxTokens:              2048,
		MaxToolIterations:      100,
		DebugTools:             false,
		EvidenceToolExposure:   true,
		PersistenceEnabled:     true,
		MaxForcedContinuations: 4,
		ResearchMinSearches:    2,
		ResearchMinFetches:     2,
		MaxSameFailureRetries:  2,
		ContextWindowFallback:  200_000,
	}
}

func applyPersistenceDefaults(cfg Config) Config {
	if cfg.MaxForcedContinuations <= 0 {
		cfg.MaxForcedContinuations = 4
	}
	if cfg.ResearchMinSearches <= 0 {
		cfg.ResearchMinSearches = 2
	}
	if cfg.ResearchMinFetches <= 0 {
		cfg.ResearchMinFetches = 2
	}
	if cfg.MaxSameFailureRetries <= 0 {
		cfg.MaxSameFailureRetries = 2
	}
	return cfg
}
