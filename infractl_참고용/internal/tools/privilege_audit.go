// Package tools
// File: privilege_audit.go
// Description: 권한 상승 감사 훅 인터페이스 정의
// Responsibility: PrivilegeAuditNotifier / PrivilegeAuditRequest 타입 선언 (순환 임포트 방지)

package tools

import "context"

// PrivilegeAuditRequest는 권한 상승 이벤트 기록에 필요한 정보이다.
type PrivilegeAuditRequest struct {
	Command       string
	NormalizedCmd string
	Invoker       string
	Method        string
	ApprovedBy    string
	// FileContents 는 pre-snapshot 용 경로→내용 맵이다.
	// 내용을 미리 읽어서 전달하거나, nil 이면 empty SHA 로 기록한다.
	FileContents map[string][]byte
}

// PrivilegeAuditNotifier는 ShellExecTool 에 주입되는 감사 콜백 인터페이스이다.
type PrivilegeAuditNotifier interface {
	// NotifyPrivilegeStart는 권한 상승 실행 직전에 호출된다. eventID를 반환한다.
	NotifyPrivilegeStart(ctx context.Context, req PrivilegeAuditRequest) string
	// NotifyPrivilegeEnd는 실행 완료 후 호출된다.
	NotifyPrivilegeEnd(ctx context.Context, eventID string, exitCode int, execLogID *int64, postContents map[string][]byte)
}
