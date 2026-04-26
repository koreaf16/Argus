// Package llm
// File: openai_inline.go
// Description: 인라인 툴 호출 메시지 변환 — vLLM tool parser 우회를 위한 메시지 이력 조작
// Responsibility: assistant/tool 메시지를 XML 기반 인라인 형식으로 변환, 클라이언트 복제

package llm

import (
	"fmt"
	"strings"
)

// SetUseInlineToolCalls는 본문 내 XML 기반 툴 호출 처리 여부를 설정한다.
func (c *OpenAIClient) SetUseInlineToolCalls(v bool) { c.useInlineToolCalls = v }

// WithInlineToolCalls는 동일 설정의 클라이언트를 복제하되 inline tool call 모드만 덮어쓴다.
// 공유 클라이언트 전역 상태를 바꾸지 않기 위해 얕은 복사본을 반환한다.
func (c *OpenAIClient) WithInlineToolCalls(v bool) Client {
	clone := *c
	clone.useInlineToolCalls = v
	return &clone
}

// IsInlineToolCalls는 현재 인라인 툴 호출 모드 활성화 여부를 반환한다.
func (c *OpenAIClient) IsInlineToolCalls() bool { return c.useInlineToolCalls }

// transformMessagesForInlineTools는 useInlineToolCalls 모드일 때
// vLLM의 내부 tool 파서를 우회하기 위해 메시지 이력을 조작한다.
func (c *OpenAIClient) transformMessagesForInlineTools(messages []Message) []Message {
	if !c.useInlineToolCalls {
		return messages
	}
	var transformed []Message
	for _, m := range messages {
		if m.Role == RoleAssistant && m.Content == "" && len(m.ToolCalls) == 0 {
			continue // invalid message — no content and no tool_calls; drop to prevent 400
		}
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			content := m.Content
			for _, tc := range m.ToolCalls {
				tag := fmt.Sprintf("\n<tool_call>\n{\"name\": %q, \"arguments\": %s}\n</tool_call>\n", tc.Function.Name, tc.Function.Arguments)
				content += tag
			}
			m.Content = strings.TrimSpace(content)
			m.ToolCalls = nil // API 스키마 검증 시 vLLM 파서 개입 방지
		} else if m.Role == RoleTool {
			// RoleTool을 RoleUser로 변경하고 <tool_response> 태그로 감싼다
			m.Role = RoleUser
			m.Content = fmt.Sprintf("<tool_response>\n%s\n</tool_response>", m.Content)
			m.ToolCallID = ""
		}
		transformed = append(transformed, m)
	}
	return transformed
}

