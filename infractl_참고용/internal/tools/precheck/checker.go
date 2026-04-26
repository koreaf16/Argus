// Package precheck
// File: checker.go
// Description: pre-check 프레임워크의 핵심 인터페이스와 결과 타입 정의
// Responsibility: Checker 인터페이스, CheckResult, CheckSummary 등 공통 계약을 선언한다.

package precheck

import (
	"context"

	"github.com/yourorg/infractl/internal/executor"
)

// CheckKind categorizes the type of pre-check being performed.
type CheckKind string

const (
	KindUserExists   CheckKind = "user_exists"
	KindBinaryExists CheckKind = "binary_exists"
	KindDirExists    CheckKind = "dir_exists"
	KindFileExists   CheckKind = "file_exists"
	KindDirWritable  CheckKind = "dir_writable"
	KindFileOwned    CheckKind = "file_owned"
	KindEnvVarSet    CheckKind = "env_var_set"
	KindDiskSpace    CheckKind = "disk_space"
	KindPortFree     CheckKind = "port_free"
	KindFsType       CheckKind = "fs_type"
	KindSudoAllowed  CheckKind = "sudo_allowed"
	KindRmSafety     CheckKind = "rm_safety"
	KindServiceExists CheckKind = "service_exists"
	KindHostReachable CheckKind = "host_reachable"
	KindNotRoot      CheckKind = "not_root"
	KindProcessExists CheckKind = "process_exists"
)

// Severity controls whether a failed check blocks execution or only warns.
type Severity int

const (
	SeverityBlock Severity = iota // 실패 시 실행 차단
	SeverityWarn                  // 실패 시 경고만, 실행은 계속
)

// CheckResult holds the outcome of a single pre-check.
type CheckResult struct {
	Kind     CheckKind
	Subject  string   // 체크 대상 (유저명, 바이너리명, 경로 등)
	OK       bool
	Message  string   // 실패 시 사용자·LLM에게 보여줄 액션 가능한 메시지
	Severity Severity
}

// CheckSummary aggregates results from all parallel checks.
type CheckSummary struct {
	Results  []CheckResult
	Blocked  bool     // SeverityBlock 체크 중 하나라도 실패하면 true
	Warnings []string // SeverityWarn 실패 메시지 목록
	BlockMsg string   // Blocked == true일 때 실행 거부 메시지
}

// Checker is the interface each pre-check type must implement.
type Checker interface {
	Kind() CheckKind
	Subject() string
	Severity() Severity
	Run(ctx context.Context, exec executor.Executor) CheckResult
}
