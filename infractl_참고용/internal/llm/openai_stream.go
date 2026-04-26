// Package llm
// File: openai_stream.go
// Description: SSE 스트림 파싱 — idle timeout, thinking/tool_call 태그 분리, 청크별 content 조립
// Responsibility: OpenAI-compatible SSE 스트림을 Response 구조체로 변환, 인라인 도구 호출 추출

package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// idleReader는 읽기마다 idle 타이머를 리셋, timeout 초과 시 cancelFn을 호출하는 io.Reader 래퍼이다.
type idleReader struct {
	r       io.Reader
	timer   *time.Timer
	timeout time.Duration
}

func newIdleReader(r io.Reader, timeout time.Duration, cancel context.CancelFunc) *idleReader {
	return &idleReader{
		r:       r,
		timeout: timeout,
		timer:   time.AfterFunc(timeout, cancel),
	}
}

// Read는 데이터 수신 시 idle 타이머를 리셋한다.
func (ir *idleReader) Read(p []byte) (int, error) {
	n, err := ir.r.Read(p)
	if n > 0 {
		ir.timer.Reset(ir.timeout)
	}
	return n, err
}

func (ir *idleReader) stop() {
	ir.timer.Stop()
}

type streamingContentGuard struct {
	raw     strings.Builder
	emitted string
}

func (g *streamingContentGuard) Push(text string) string {
	if text == "" {
		return ""
	}
	g.raw.WriteString(text)
	if hasUnclosedThinkingTag(g.raw.String()) || hasDanglingThinkingTagSuffix(g.raw.String()) {
		return ""
	}
	safe := sanitizeAssistantContent(g.raw.String())
	if safe == "" {
		return ""
	}
	rawTrimmed := strings.TrimSpace(strings.ReplaceAll(g.raw.String(), "\r\n", "\n"))
	if hasPossibleReasoningLeakPrefix(g.raw.String()) && safe == rawTrimmed {
		return ""
	}
	if strings.HasPrefix(safe, g.emitted) {
		delta := safe[len(g.emitted):]
		g.emitted = safe
		return delta
	}
	delta := safe
	g.emitted = safe
	return delta
}

func (c *OpenAIClient) parseStream(model string, body io.Reader, onThinkingToken func(string), onToken func(string)) (Response, error) {
	var thinkBuf strings.Builder
	var contentBuf strings.Builder
	var contentGuard streamingContentGuard
	var inlineToolBuf strings.Builder // <tool_call> 태그 내용을 담을 버퍼
	toolCallBuf := make(map[int]*ToolCall)
	var inputTokens, outputTokens int

	const (
		tagToolStart     = "<tool_call>"
		tagToolEnd       = "</tool_call>"
		tagToolCodeStart = "<tool_code>"
		tagToolCodeEnd   = "</tool_code>"
	)

	// thinking 태그 변형: <think>, <thinking>, <thought>
	thinkStarts := [3]string{"<think>", "<thinking>", "<thought>"}
	thinkEnds := [3]string{"</think>", "</thinking>", "</thought>"}

	// 상태 정의
	const (
		stateContent = iota
		stateThinking
		stateToolCalling
	)

	state := stateContent
	pendingBuf := ""
	curThinkEnd := thinkEnds[0] // stateThinking 진입 시 매칭 종료 태그로 갱신됨
	curToolEnd := tagToolEnd    // stateToolCalling 진입 시 매칭 종료 태그로 갱신됨

	flushPending := func(text string) {
		if text == "" {
			return
		}
		switch state {
		case stateThinking:
			thinkBuf.WriteString(text)
			if onThinkingToken != nil {
				onThinkingToken(text)
			}
		case stateToolCalling:
			inlineToolBuf.WriteString(text)
		default:
			contentBuf.WriteString(text)
			if onToken != nil {
				if safeDelta := contentGuard.Push(text); safeDelta != "" {
					onToken(safeDelta)
				}
			}
		}
	}

	processContent := func(incoming string) {
		pendingBuf += incoming
		for {
			switch state {
			case stateThinking:
				idx := strings.Index(pendingBuf, curThinkEnd)
				if idx >= 0 {
					flushPending(pendingBuf[:idx])
					state = stateContent
					pendingBuf = strings.TrimLeft(pendingBuf[idx+len(curThinkEnd):], "\n")
				} else {
					safeLen := len(pendingBuf) - (len(curThinkEnd) - 1)
					if safeLen > 0 {
						flushPending(pendingBuf[:safeLen])
						pendingBuf = pendingBuf[safeLen:]
					}
					return
				}

			case stateToolCalling:
				// curToolEnd(</tool_call> 또는 </tool_code>) 또는 </call:NAME> 탐색
				idx := strings.Index(pendingBuf, curToolEnd)
				tagLen := len(curToolEnd)
				if idx < 0 {
					// </call:NAME> 형식 탐색
					if cIdx := strings.Index(pendingBuf, "</call:"); cIdx >= 0 {
						if eIdx := strings.Index(pendingBuf[cIdx:], ">"); eIdx >= 0 {
							idx = cIdx
							tagLen = eIdx + 1
						}
					}
				}

				if idx >= 0 {
					inlineToolBuf.WriteString(pendingBuf[:idx+tagLen])
					state = stateContent
					pendingBuf = pendingBuf[idx+tagLen:]
				} else {
					// 닫는 태그가 아직 완성되지 않았을 수 있으므로 안전한 길이만큼만 버퍼로 이동
					safeLen := len(pendingBuf) - 30 // 넉넉하게 잡음 (</call:very_long_name_here>)
					if safeLen > 0 {
						inlineToolBuf.WriteString(pendingBuf[:safeLen])
						pendingBuf = pendingBuf[safeLen:]
					}
					return
				}

			default: // stateContent
				// 가장 먼저 등장하는 thinking 시작 태그(<think>/<thinking>/<thought>) 탐색
				thinkIdx := -1
				thinkStartLen := 0
				for i, startTag := range thinkStarts {
					idx := strings.Index(pendingBuf, startTag)
					if idx >= 0 && (thinkIdx < 0 || idx < thinkIdx) {
						thinkIdx = idx
						thinkStartLen = len(startTag)
						curThinkEnd = thinkEnds[i]
					}
				}
				toolIdx := -1
				curToolEnd = tagToolEnd
				if c.useInlineToolCalls {
					toolIdx = strings.Index(pendingBuf, tagToolStart)
					if codeIdx := strings.Index(pendingBuf, tagToolCodeStart); codeIdx >= 0 {
						if toolIdx < 0 || codeIdx < toolIdx {
							toolIdx = codeIdx
							curToolEnd = tagToolCodeEnd
						}
					}
					if callIdx := strings.Index(pendingBuf, "call:"); callIdx >= 0 {
						if toolIdx < 0 || callIdx < toolIdx {
							toolIdx = callIdx
						}
					}
				}

				if thinkIdx >= 0 && (toolIdx < 0 || thinkIdx < toolIdx) {
					flushPending(pendingBuf[:thinkIdx])
					state = stateThinking
					pendingBuf = pendingBuf[thinkIdx+thinkStartLen:]
				} else if toolIdx >= 0 {
					flushPending(pendingBuf[:toolIdx])
					state = stateToolCalling
					// 시작 태그(또는 call: 접두사)부터 버퍼링 시작
					// pendingBuf는 이미 toolIdx 이후를 가리키게 되므로
					// 아래 루프 시작 시 pendingBuf에서 내용을 가져오게 됨
					pendingBuf = pendingBuf[toolIdx:]
				} else {
					maxTagLen := 12 // len("</tool_call>")
					safeLen := len(pendingBuf) - (maxTagLen - 1)
					if safeLen > 0 {
						flushPending(pendingBuf[:safeLen])
						pendingBuf = pendingBuf[safeLen:]
					}
					return
				}
			}
		}
	}

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		if payload == "" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			inputTokens = chunk.Usage.PromptTokens
			outputTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Reasoning != "" {
			thinkBuf.WriteString(delta.Reasoning)
			if onThinkingToken != nil {
				onThinkingToken(delta.Reasoning)
			}
		}
		if delta.Content != "" {
			processContent(delta.Content)
		}
		for _, tc := range delta.ToolCalls {
			accumulateToolCall(toolCallBuf, tc)
		}
	}

	flushPending(pendingBuf)
	if err := scanner.Err(); err != nil {
		return Response{}, fmt.Errorf("read stream: %w", err)
	}

	toolCalls := assembleToolCalls(toolCallBuf)
	rawContent := contentBuf.String()
	finalContent := sanitizeAssistantContent(rawContent)

	// 1. API(tool_calls)로부터 받은 것 필터링
	filtered := toolCalls[:0]
	for _, tc := range toolCalls {
		if tc.Function.Name != "" {
			filtered = append(filtered, tc)
		}
	}
	toolCalls = filtered

	// 2. 본문 내 인라인 추출 병합
	if inlineStr := inlineToolBuf.String(); inlineStr != "" {
		slog.Debug("parseStream: inlineToolBuf found", "content", inlineStr)
		if parsed, _ := extractInlineToolCalls(inlineStr); len(parsed) > 0 {
			slog.Debug("parseStream: inline tool calls extracted", "count", len(parsed))
			toolCalls = append(toolCalls, parsed...)
		}
	}

	// 3. Fallback: 상태 머신이 놓친 경우 대비 (rawContent에서 체크 — sanitize 전이라 태그가 남아 있음)
	if len(toolCalls) == 0 {
		if hasInlineToolCallCandidate(rawContent) {
			slog.Debug("parseStream: fallback content check", "found", true)
			if parsed, cleaned := extractInlineToolCalls(rawContent); len(parsed) > 0 {
				toolCalls = parsed
				finalContent = sanitizeAssistantContent(cleaned)
			}
		}
		// thinking 영역에 포함된 경우도 체크 (일부 모델은 thought 내에 툴 호출을 생성하기도 함)
		thinkContent := thinkBuf.String()
		if len(toolCalls) == 0 && hasInlineToolCallCandidate(thinkContent) {
			slog.Debug("parseStream: fallback think check", "found", true)
			if parsed, _ := extractInlineToolCalls(thinkContent); len(parsed) > 0 {
				toolCalls = parsed
			}
		}
	}

	if len(toolCalls) > 0 {
		slog.Info("parseStream: tool calls detected", "count", len(toolCalls))
	}

	logToFile(model, "RESPONSE", finalContent)
	logToolCalls(model, toolCalls, inlineToolBuf.String())
	return Response{
		Content:      finalContent,
		ToolCalls:    toolCalls,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}
