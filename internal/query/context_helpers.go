// 파일 역할: engine 내 context management 보조 함수.
package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/llm"
)

// makeSummarizeFn 은 engine의 LLM을 사용해 tool output을 요약하는 함수를 반환한다.
// distiller에 주입되어 LLM 요약 단계에서 사용된다.
func (e *Engine) makeSummarizeFn() func(toolName, content string) (string, error) {
	return func(toolName, content string) (string, error) {
		e.mu.RLock()
		client := e.llm
		e.mu.RUnlock()

		if client == nil {
			return "", fmt.Errorf("llm not configured")
		}

		prompt := fmt.Sprintf(
			`다음은 '%s' 도구의 출력입니다. 이 내용에서 모델이 컨텍스트를 잃지 않도록 핵심 정보만 10줄 이내로 추출해주세요.

집중할 사항:
1. 정확한 에러 메시지, 예외 타입, 종료 코드
2. 구체적인 파일 경로 또는 줄 번호
3. 확정적인 결과 (예: '컴파일 성공', '테스트 3개 실패')

출력 내용 (앞부분):
%s`, toolName, content[:min(len(content), 8000)])

		// Use a capped context so a slow LLM can't block distillation indefinitely.
		sCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		stream, err := client.Stream(sCtx, llm.Request{
			Messages: []llm.Message{llm.TextMessage(llm.RoleUser, prompt)},
		})
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		for evt := range stream {
			if evt.Kind == llm.EventTextDelta {
				sb.WriteString(evt.Delta)
			}
			if evt.Kind == llm.EventError {
				return "", evt.Err
			}
		}
		return sb.String(), nil
	}
}

// estimateSystemTokens 는 system blocks의 추정 토큰 수를 반환한다.
func estimateSystemTokens(blocks []llm.SystemBlock) int {
	total := 0
	for _, b := range blocks {
		total += len(b.Text)
	}
	return total / 4
}

// min 은 두 정수 중 작은 값을 반환한다 (Go 1.22 이하 호환).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
