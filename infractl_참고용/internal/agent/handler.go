// Package agent
// File: handler.go
// Description: 에이전트 루프 이벤트를 UI로 전달하는 핸들러 인터페이스 정의
// Responsibility: 에이전트와 UI 레이어 간 이벤트 통신 인터페이스 제공

package agent

import (
	"context"
	"time"

	"github.com/yourorg/infractl/internal/tools"
)

// EventHandler는 에이전트 루프의 이벤트를 UI로 전달하는 인터페이스이다.
// Phase 1: cli.REPLHandler가 구현
// Phase 3: TUI 핸들러가 구현
// Phase 7: Web UI 핸들러가 구현
type EventHandler interface {
	// OnThinking은 LLM이 응답 생성을 시작했을 때 호출된다.
	// tier는 사용 중인 LLM 티어 ("reasoning", "general", "fast"),
	// model은 모델명 (예: "qwen-35b")이다.
	OnThinking(tier string, model string)

	// OnThinkingToken은 LLM의 내부 추론(thinking) 토큰을 받았을 때 호출된다.
	// </think> 이전까지의 스트리밍 토큰이 전달된다.
	OnThinkingToken(token string)

	// OnToken은 LLM 스트리밍 응답의 토큰 하나를 받았을 때 호출된다.
	OnToken(token string)

	// OnToolStart는 도구 실행이 시작될 때 호출된다.
	// toolID는 LLM이 부여한 도구 호출 식별자 (병렬 실행 라우팅용).
	// target은 실행 대상 서버 이름 ("" 또는 "localhost"이면 로컬).
	OnToolStart(toolID string, toolName string, target string, args map[string]any)

	// OnToolOutput은 도구 실행 중 stdout 라인을 수신했을 때 호출된다.
	// shell_exec 등 스트리밍 출력을 지원하는 도구에서 사용된다.
	OnToolOutput(toolID string, line string)

	// OnToolEnd는 도구 실행이 완료되었을 때 호출된다.
	OnToolEnd(toolID string, toolName string, result string, duration time.Duration, success bool, metadataJSON string)

	// OnToolCancelled는 사용자에 의해 도구 실행이 취소되었을 때 호출된다.
	OnToolCancelled(toolID string, toolName string, reason string, duration time.Duration)

	// OnResponse는 LLM의 최종 텍스트 응답이 완성되었을 때 호출된다.
	OnResponse(content string)

	// OnError는 에이전트 루프에서 복구 불가능한 에러가 발생했을 때 호출된다.
	OnError(err error)

	// OnUsageUpdate는 LLM 호출 후 토큰/비용 정보를 전달한다.
	OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, durationMs int64)

	// OnJobComplete는 백그라운드 작업이 완료되었을 때 호출된다.
	OnJobComplete(jobID int, description string, success bool)

	// OnRAGContext는 내부 지식(knowledge/learned_system)이 시스템 프롬프트에 주입될 때 호출된다.
	// count는 주입된 항목 수이다.
	OnRAGContext(count int)

}

// IdleInputRequest는 명령 실행 중 유휴 상태가 감지되었을 때의 요청 정보이다.
type IdleInputRequest struct {
	ToolName  string   // 실행 중인 도구 이름
	Target    string   // 실행 대상 서버
	Command   string   // 실행 중인 명령
	LastLines []string // 유휴 직전 마지막 stdout 라인들 (컨텍스트 제공용)
}

// IdleInputResponse는 인터랙티브 프롬프트에 대한 응답이다.
type IdleInputResponse struct {
	Input      string // stdin에 쓸 내용. 빈 문자열이면 빈 줄(Enter)을 전송한다.
	CloseStdin bool   // true이면 stdin에 EOF/EOT를 보낸다.
	Abort      bool   // true이면 프로세스를 종료한다.
}

// IdleInputHandler는 명령 실행 중 유휴(인터랙티브 프롬프트 대기) 상태를 처리한다.
// SmartIdleInputHandler(LLM 판단 + TUI 폴백)가 기본 구현이다.
type IdleInputHandler interface {
	// RequestIdleInput은 유휴 상태에 대한 응답을 결정하고 반환한다.
	// ctx가 취소되면 Abort=true인 IdleInputResponse를 반환한다.
	RequestIdleInput(ctx context.Context, req IdleInputRequest) (IdleInputResponse, error)
}

// QuestionHandler는 다중 선택형 질의응답을 처리하는 인터페이스이다.
// TUIHandler가 구현하며, Agent가 이를 통해 도구 호출 시 UI와 통신한다.
type QuestionHandler interface {
	// RequestQuestion은 사용자에게 질문을 던지고 답변을 대기한다.
	RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error)
	// RequestForm은 사용자에게 다중 필드 폼 입력을 요청하고 결과를 대기한다.
	RequestForm(ctx context.Context, req tools.FormRequest) (tools.FormResponse, error)
}

// TaskEventSink is an optional extension of EventHandler for task lifecycle events.
// Handlers that support task context display implement this interface.
// Agent checks with type assertion: if s, ok := a.handler.(TaskEventSink); ok { ... }
type TaskEventSink interface {
	// OnTaskProposed is called when propose_task creates a PendingProposal.
	OnTaskProposed(title, kind, server, account string)
	// OnTaskDeclared is called when a TaskContext is declared.
	OnTaskDeclared(taskID, title, kind string)
	// OnTaskStepAdvanced is called when the LLM advances to a new plan step.
	OnTaskStepAdvanced(taskID string, stepIndex, total int, description string)
	// OnTaskEnded is called when the task ends (completed/failed/aborted).
	OnTaskEnded(taskID, status, summary string)
	// OnElevationChanged is called when the current user changes in a session.
	OnElevationChanged(host, sessionID, currentUser string)
	// OnGuardViolation is called when TaskGuard blocks or warns about a tool call.
	OnGuardViolation(toolName, reason string, blocked bool)
}
