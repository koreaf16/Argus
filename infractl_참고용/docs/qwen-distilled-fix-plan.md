# Qwen Distilled Mode 응답 생성 실패 수정 계획

## 문제 개요
Qwen(Distilled 모드) 모델이 도구(`server_focus` 등)를 호출하고 정상적으로 그 결과를 받았음에도 불구하고, 이후 텍스트 응답을 생성하지 못해 `응답을 생성하지 못했습니다.` 에러가 발생하는 문제.

## 원인 분석
이 문제는 크게 파싱 버퍼 크기 부족(스트리밍)과 모델에 대한 도구 결과 처리 지시 누락 두 가지 원인으로 발생합니다.

### 1. 스트리밍 파서 버퍼 크기 버그 (`internal/llm/openai.go`)
- `openai.go` 내의 `processContent` 함수에서 `c.useInlineToolCalls` 활성화 시, 본문 내 `<tool_call>` 태그를 가로채기 위해 스트림 버퍼(`pendingBuf`)를 사용합니다.
- 이때 버퍼 길이를 제한하는 `maxTagLen`이 `8` (len("<think>") + 1)로 하드코딩되어 있습니다.
- `<tool_call>` 태그는 11바이트이므로, 스트리밍 청크가 하나씩 들어올 때 `pendingBuf`에 8바이트 이상이 쌓이면 즉시 `finalContent`로 플러시되어 버립니다.
- 이로 인해 실시간 파싱이 완전히 실패하며, 응답 전체가 다 받아진 이후의 `Fallback` 로직에서만 도구를 추출하게 됩니다. 만약 모델이 `<tool_call>` 외의 추가 출력을 하지 않으면, Fallback 추출 후 `finalContent`가 빈 문자열이 되어 빈 응답 에러의 원인이 되기도 합니다.

### 2. 도구 결과물(`tool_response`)에 대한 프롬프트 지시 누락 (`internal/agent/prompt_tools.go`)
- 현재 시스템 프롬프트에는 모델에게 `Distilled Mode`에서 어떻게 도구(`tool_call`)를 호출해야 하는지에 대한 포맷(`XML`)만 명시되어 있습니다.
- 도구 실행 후 시스템이 그 결과를 `RoleUser`의 `<tool_response>결과내용</tool_response>` 형태로 다시 모델에게 주입하는데, 프롬프트에는 `<tool_response>`를 어떻게 처리해야 하는지 지시가 없습니다.
- 때문에 Qwen 모델은 사용자가 `<tool_response>`라는 XML을 입력한 것으로 인식하고, 해당 도구 결과를 토대로 최종 텍스트 답변을 해야 한다는 것을 알지 못해 답변 생성을 멈춰버리거나 빈 응답을 반환합니다.

## 해결 방법

### 1. `maxTagLen` 상향 조정
`internal/llm/openai.go` 의 `processContent` 함수:
```go
				} else {
					maxTagLen := 12 // len("</tool_call>")
					safeLen := len(pendingBuf) - (maxTagLen - 1)
					if safeLen > 0 {
						flushPending(pendingBuf[:safeLen])
						pendingBuf = pendingBuf[safeLen:]
					}
					return
				}
```
위와 같이 `maxTagLen`을 12로 변경하여 `<tool_call>` (11바이트) 태그 전체가 버퍼에 온전히 수신될 때까지 스트리밍 버퍼가 대기하도록 합니다.

### 2. 프롬프트 명시적 지시 추가
`internal/agent/prompt_tools.go` 의 `appendToolCallingFormat` 함수:
```go
	sb.WriteString("## Tool Calling Format (Qwen Distilled Mode)\n")
	sb.WriteString("You are running in a special distilled mode. To use tools, you MUST output them in the following XML format directly in your response body:\n\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString("{\"name\": \"tool_name\", \"arguments\": {\"arg1\": \"value1\"}}\n")
	sb.WriteString("</tool_call>\n\n")
	sb.WriteString("Points to remember:\n")
	sb.WriteString("- Do not use standard markdown code blocks for tools.\n")
	sb.WriteString("- You can call multiple tools by repeating the <tool_call> block.\n")
	sb.WriteString("- Output your thought process (Thinking) first, then the tool call.\n")
	sb.WriteString("- After the system executes your tool, it will reply with a <tool_response> block.\n")
	sb.WriteString("- You MUST analyze the <tool_response> and provide a final text response to the user in Korean.\n")
	sb.WriteString("- Do NOT output empty responses.\n\n")
```
위와 같이 `<tool_response>` 수신 후 사용자에게 반드시 한국어로 최종 텍스트 결과를 전달해야 한다는 지시사항을 추가합니다.
