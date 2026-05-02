package query

import "sync"

type TokenBudget struct {
	mu                sync.Mutex
	Input             int
	Output            int
	Thinking          int
	CumulativeInput   int
	CumulativeOutput  int
	CumulativeThinking int
}

func NewTokenBudget() *TokenBudget {
	return &TokenBudget{}
}

func (b *TokenBudget) AddInput(n int) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	b.Input += n
	b.CumulativeInput += n
	b.mu.Unlock()
}

func (b *TokenBudget) AddOutput(n int) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	b.Output += n
	b.CumulativeOutput += n
	b.mu.Unlock()
}

func (b *TokenBudget) AddThinking(n int) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	b.Thinking += n
	b.CumulativeThinking += n
	b.mu.Unlock()
}

func (b *TokenBudget) Snapshot() (input, output, thinking int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Input, b.Output, b.Thinking
}

func (b *TokenBudget) CumulativeSnapshot() (input, output, thinking int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.CumulativeInput, b.CumulativeOutput, b.CumulativeThinking
}

func (b *TokenBudget) Reset() {
	b.mu.Lock()
	b.Input = 0
	b.Output = 0
	b.Thinking = 0
	b.mu.Unlock()
}
