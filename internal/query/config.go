package query

type Config struct {
	MaxTokens         int
	MaxToolIterations int
	DebugTools        bool
	// ContextWindowFallback — activeModelContextWindow()가 0을 반환할 때 사용할 기본값
	ContextWindowFallback int
}

func DefaultConfig() Config {
	return Config{
		MaxTokens:             2048,
		MaxToolIterations:     100,
		DebugTools:            false,
		ContextWindowFallback: 200_000,
	}
}

