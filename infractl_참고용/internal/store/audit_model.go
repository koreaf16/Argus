// Package store
// File: audit_model.go
// Description: 감사(Audit) 레이어 도메인 모델 정의
// Responsibility: audit_runs / audit_run_plans / audit_planned_steps / audit_steps / audit_checks 구조체

package store

import "time"

// AuditRunKind는 작업 종류를 나타낸다.
type AuditRunKind string

const (
	AuditRunKindInstall     AuditRunKind = "install"
	AuditRunKindSetup       AuditRunKind = "setup"
	AuditRunKindHealthcheck AuditRunKind = "healthcheck"
	AuditRunKindMaintenance AuditRunKind = "maintenance"
	AuditRunKindAdHoc       AuditRunKind = "ad-hoc"
)

// AuditRunStatus는 작업 진행 상태를 나타낸다.
type AuditRunStatus string

const (
	AuditRunStatusPlanning  AuditRunStatus = "planning"
	AuditRunStatusRunning   AuditRunStatus = "running"
	AuditRunStatusSucceeded AuditRunStatus = "succeeded"
	AuditRunStatusFailed    AuditRunStatus = "failed"
	AuditRunStatusAborted   AuditRunStatus = "aborted"
)

// AuditRun은 audit_runs 테이블의 단일 행이다.
// 다단계 작업 세션의 헤더 역할을 한다.
type AuditRun struct {
	ID           string
	ServerID     string
	ServerName   string
	Kind         AuditRunKind
	Title        string
	UserPrompt   string
	ActivePlanID string // NULL이면 ""
	Status       AuditRunStatus
	StartedAt    time.Time
	EndedAt      *time.Time
	SummaryMD    string
	MetadataJSON string
}

// AuditPlanAuthor는 계획서 작성 주체를 나타낸다.
type AuditPlanAuthor string

const (
	AuditPlanAuthorUser    AuditPlanAuthor = "user"
	AuditPlanAuthorLLM     AuditPlanAuthor = "llm"
	AuditPlanAuthorUserLLM AuditPlanAuthor = "user+llm"
)

// AuditRunPlan은 audit_run_plans 테이블의 단일 행이다.
// 계획서는 append-only이며 revision 번호로 개정 이력을 관리한다.
type AuditRunPlan struct {
	ID          string
	RunID       string
	Revision    int
	Author      AuditPlanAuthor
	GoalMD      string
	RationaleMD string
	PlanMD      string
	RiskMD      string
	ApprovedBy  string
	ApprovedAt  *time.Time
	CreatedAt   time.Time
}

// AuditPlannedStep은 audit_planned_steps 테이블의 단일 행이다.
type AuditPlannedStep struct {
	ID           string
	PlanID       string
	Seq          int
	Label        string
	IntentMD     string
	ExpectedCmds string
	RollbackMD   string
}

// AuditStepStatus는 실행 스텝의 상태를 나타낸다.
type AuditStepStatus string

const (
	AuditStepStatusPending   AuditStepStatus = "pending"
	AuditStepStatusRunning   AuditStepStatus = "running"
	AuditStepStatusOK        AuditStepStatus = "ok"
	AuditStepStatusFail      AuditStepStatus = "fail"
	AuditStepStatusSkipped   AuditStepStatus = "skipped"
	AuditStepStatusUnplanned AuditStepStatus = "unplanned"
)

// AuditStep은 audit_steps 테이블의 단일 행이다.
// execution_logs와 연결되어 실제 도구 실행 이력을 참조한다.
type AuditStep struct {
	ID            string
	RunID         string
	PlannedStepID string // NULL이면 ""
	Seq           int
	Label         string
	ExecLogID     *int64
	Status        AuditStepStatus
	ResultMD      string
	DeviationMD   string
	StartedAt     time.Time
	EndedAt       *time.Time
}

// AuditCheckVerdict는 점검 결과 판정이다.
type AuditCheckVerdict string

const (
	AuditCheckVerdictOK   AuditCheckVerdict = "ok"
	AuditCheckVerdictWarn AuditCheckVerdict = "warn"
	AuditCheckVerdictFail AuditCheckVerdict = "fail"
	AuditCheckVerdictInfo AuditCheckVerdict = "info"
)

// AuditCheck는 audit_checks 테이블의 단일 행이다.
type AuditCheck struct {
	ID       string
	RunID    string // NULL이면 ""
	ServerID string
	ServerName string
	Category string
	Name     string
	Verdict  AuditCheckVerdict
	RawOutput string
	FindingsMD string
	CheckedAt time.Time
}

// AuditRunSummary는 브리핑·리스트 조회용 경량 뷰이다.
type AuditRunSummary struct {
	ID          string
	ServerName  string
	Kind        AuditRunKind
	Title       string
	Status      AuditRunStatus
	StepTotal   int
	StepDone    int
	LastStepLbl string
	StartedAt   time.Time
	EndedAt     *time.Time
}

// AuditRunDetail은 audit show 조회용 상세 뷰이다.
type AuditRunDetail struct {
	Run         AuditRun
	ActivePlan  *AuditRunPlan
	Steps       []AuditStep
	PlannedSteps []AuditPlannedStep
}
