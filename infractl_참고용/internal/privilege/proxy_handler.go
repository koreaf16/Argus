package privilege

import (
	"context"
	"sync"
)

// ProxyPromptHandler는 권한 프롬프트 요청을 위임하는 프록시이다.
type ProxyPromptHandler struct {
	mu      sync.RWMutex
	handler PromptHandler
}

// Set은 내부 핸들러를 설정한다.
func (p *ProxyPromptHandler) Set(h PromptHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

// RequestPassword는 핸들러에게 권한 암호 요청을 위임한다.
func (p *ProxyPromptHandler) RequestPassword(ctx context.Context, req PromptRequest) (PromptResponse, error) {
	p.mu.RLock()
	h := p.handler
	p.mu.RUnlock()

	if h == nil {
		return PromptResponse{Abort: true}, nil
	}
	return h.RequestPassword(ctx, req)
}
