// Package hooks — settings.json 기반 사용자 정의 훅 시스템.
//
// 파일 역할: HookCommand, HookMatcher, HooksConfig, HookInput, HookExecResult,
//
//	AggregatedResult 타입 정의.
//
// 연결:
//   - 원본: src/schemas/hooks.ts, src/types/hooks.ts
package hooks

import (
	"context"

	"github.com/koreaf16/argus/internal/types"
)

// HookCommandType 은 훅 명령 종류다.
type HookCommandType string

const (
	HookCommandTypeCommand HookCommandType = "command"
	HookCommandTypeHTTP    HookCommandType = "http"
	HookCommandTypePrompt  HookCommandType = "prompt"
	// Phase 4: agent
)

// HookCommand 는 단일 훅 명령 정의다.
type HookCommand struct {
	Type          HookCommandType `json:"type"`
	Command       string          `json:"command,omitempty"`
	Shell         string          `json:"shell,omitempty"`   // "bash" | "powershell"
	Timeout       int             `json:"timeout,omitempty"` // seconds, 기본 60
	StatusMessage string          `json:"statusMessage,omitempty"`

	// 공통 제어 플래그
	Once  bool   `json:"once,omitempty"`  // 세션당 1회만 실행
	Async bool   `json:"async,omitempty"` // 비동기 실행 (AggregatedResult 영향 없음)
	If    string `json:"if,omitempty"`    // 조건 필터: "Bash(git *)" permission rule 구문

	// http 타입 전용
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	AllowedEnvVars []string          `json:"allowedEnvVars,omitempty"`

	// prompt 타입 전용
	Prompt string `json:"prompt,omitempty"` // $ARGUMENTS 치환 지원
	Model  string `json:"model,omitempty"`  // 기본: 엔진 기본 모델
}

// SubQueryFn 은 LLM에 단일 턴 쿼리를 실행하는 함수 시그니처다.
type SubQueryFn func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// HookMatcher 는 이벤트 내에서 특정 도구/패턴을 매칭하는 단위다.
type HookMatcher struct {
	Matcher string        `json:"matcher,omitempty"` // 도구 이름, 빈 문자열=전체 매칭
	Hooks   []HookCommand `json:"hooks"`
}

// HooksConfig 는 settings.json의 "hooks" 필드 전체 구조다.
// key = HookEvent 이름, value = []HookMatcher
type HooksConfig map[types.HookEvent][]HookMatcher

// HookInput 은 훅 실행 시 주입되는 컨텍스트 데이터다.
type HookInput struct {
	SessionID     string
	WorkingDir    string
	ToolName      string // PreToolUse / PostToolUse
	ToolInputJSON string // JSON 직렬화된 도구 입력
	ToolResult    string // PostToolUse 결과 텍스트
	IsToolError   bool   // PostToolUseFailure 시 true
	UserPrompt    string // UserPromptSubmit 시 원본 프롬프트
}

// HookExecResult 는 단일 command 훅 실행 결과다.
type HookExecResult struct {
	ExitCode     int
	Stdout       string
	Stderr       string
	Continue     bool           // 기본 true, false = 실행 차단
	StopReason   string         // continue=false 일 때 사유
	Decision     string         // "approve" | "block" | ""
	Reason       string         // decision 사유
	UpdatedInput map[string]any // PreToolUse 에서 입력 수정 시
}

// AggregatedResult 는 이벤트에 매칭된 모든 훅의 집계 실행 결과다.
type AggregatedResult struct {
	Continue     bool
	StopReason   string
	BlockReason  string         // exit 2 또는 {"decision":"block"} 시 사유
	UpdatedInput map[string]any // 마지막 훅의 updatedInput
	Warnings     []string       // exit code != 0, 2 케이스의 stderr
	Stdout       string         // exit 0 시 LLM 컨텍스트로 전달할 stdout
}
