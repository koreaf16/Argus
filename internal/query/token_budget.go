package query

import "sync"

type TokenBudget struct {
	mu     sync.Mutex
	Input  int
	Output int
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
	b.mu.Unlock()
}

func (b *TokenBudget) AddOutput(n int) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	b.Output += n
	b.mu.Unlock()
}

func (b *TokenBudget) Snapshot() (input, output int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Input, b.Output
}

func (b *TokenBudget) Reset() {
	b.mu.Lock()
	b.Input = 0
	b.Output = 0
	b.mu.Unlock()
}
