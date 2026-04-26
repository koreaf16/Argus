// Package taskctx
// File: context.go
// Description: 복합 작업 단위의 선언적 컨텍스트
// Responsibility: TaskContext 구조체와 불변 필드 접근자

package taskctx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/yourorg/infractl/internal/privilege"
)

// TaskStatus 는 작업의 생명주기 상태이다.
type TaskStatus string

const (
	TaskActive    TaskStatus = "active"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskAborted   TaskStatus = "aborted"
)

// TaskKind 는 작업의 유형을 나타낸다.
type TaskKind string

const (
	KindInstall   TaskKind = "install"
	KindService   TaskKind = "service"
	KindConfigure TaskKind = "configure"
	KindMigrate   TaskKind = "migrate"
	KindBackup    TaskKind = "backup"
	KindPatch     TaskKind = "patch"
	KindInspect   TaskKind = "inspect"
	KindOther     TaskKind = "other"
)

// TaskNote 는 작업 진행 중 발생한 주목할 만한 이벤트 기록이다.
type TaskNote struct {
	At      time.Time
	Kind    string // "precondition_met"|"guard_violation"|"elevation"|"step_advance"
	Message string
}

// TaskContext 는 한 복합 작업 단위의 선언적 컨텍스트.
// Plan 은 단계 설명 문자열 슬라이스로만 보관 — 실제 PlanState 는 agent 가 별도로 소유.
type TaskContext struct {
	ID            string
	Title         string
	Kind          TaskKind
	TargetServer  string
	TargetAccount string
	WorkingDir    string
	Forbidden     []string          // literal prefix 또는 정규식 (guardrail.go가 컴파일)
	Preconditions []string          // 자연어 기술
	PlanSteps     []string          // 단계 설명 슬라이스
	KindFields    map[string]string // Kind 별 추가 필드 — map 으로 유연 보관
	// Elevation 은 런타임 참조 (privilege 패키지). 선언 시점에는 nil.
	Elevation *privilege.ElevationState
	Notes     []TaskNote
	CreatedAt time.Time
	EndedAt   *time.Time
	Status    TaskStatus
}

// NewID 는 태스크 ID 생성 — 단순 hex 난수 16자리 (crypto/rand)
func NewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// fallback: timestamp-based (extremely unlikely to fail)
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Summary 는 한 줄 요약 "Task · server · account · dir"
func (t *TaskContext) Summary() string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("Task · %s · %s · %s", t.TargetServer, t.TargetAccount, t.WorkingDir)
}

// AppendNote 는 thread-safe 하지 않음 — Manager 가 락 잡고 호출
func (t *TaskContext) AppendNote(kind, msg string) {
	t.Notes = append(t.Notes, TaskNote{
		At:      time.Now(),
		Kind:    kind,
		Message: msg,
	})
}
