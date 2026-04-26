// Package subagent
// File: types.go
// Description: 서브에이전트 타입 상수 및 요청/결과 타입 정의
// Responsibility: AgentType 분류, SubagentConfig, SubagentResult 타입 제공

package subagent

// AgentType은 서브에이전트 전문 분야를 나타낸다.
type AgentType string

const (
	AgentTypeDB       AgentType = "db"       // 데이터베이스 분석 (Oracle, MySQL, PostgreSQL)
	AgentTypeOS       AgentType = "os"       // 운영체제 분석 (프로세스, 파일, 네트워크)
	AgentTypeWAS      AgentType = "was"      // WAS 분석 (Tomcat, WebLogic)
	AgentTypeSecurity AgentType = "security" // 보안 상태 분석
	AgentTypeIntel    AgentType = "intel"    // Pre-flight 인텔리전스 수집 (reasoning 작업 전 환경 파악)
	AgentTypeHook     AgentType = "hook"     // Hook 실행 및 판단 전용 (웹 검색 강제 없음)
)

// AllAgentTypes는 기본 서브에이전트 타입 집합이다.
// AgentTypeIntel은 포함하지 않는다 — reasoning 전용으로 별도 호출된다.
var AllAgentTypes = []AgentType{AgentTypeDB, AgentTypeOS, AgentTypeWAS, AgentTypeSecurity}

// SubagentConfig는 단일 서브에이전트 실행 설정이다.
type SubagentConfig struct {
	Type     AgentType
	Server   string // 대상 서버명
	Question string // 분석 질문
}

// SubagentResult는 단일 서브에이전트 실행 결과이다.
type SubagentResult struct {
	Type         AgentType
	Server       string
	Answer       string
	Err          error
	ToolUses     int
	InputTokens  int
	OutputTokens int
}
