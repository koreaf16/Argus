// Package shell
// File: provider.go
// Description: ShellProvider 인터페이스 정의
// Responsibility: 쉘 종류별 명령어 래핑·분석 계약 명세

// Ported from: claude_cli/src/utils/shell/shellProvider.ts:1-34

package shell

import "context"

// ShellProvider 는 특정 쉘에서 명령어를 준비·분석하는 추상이다.
// 각 구현(bash, powershell)은 명령어를 해당 쉘 문법으로 래핑하고 위험도를 분석한다.
type ShellProvider interface {
	// Name 은 쉘 식별자를 반환한다 (예: "bash", "powershell").
	Name() string

	// Wrap 은 원시 명령어를 쉘 실행 형식으로 변환한다.
	// 내부적으로 Prepare 를 호출하며, 기존 호환성을 위해 유지된다.
	// Phase E 완료 후 호출처를 Prepare 로 점진 이전한다.
	Wrap(ctx context.Context, command string) (string, error)

	// Prepare 는 명령어를 exec.Command 에 전달 가능한 PreparedCmd 로 변환한다.
	// quoting, snapshot sourcing, glob 비활성화, PowerShell 인코딩 등을 수행한다.
	// Ported from: claude_cli/src/utils/shell/bashProvider.ts:buildExecCommand
	Prepare(ctx context.Context, command string) (PreparedCmd, error)

	// Analyze 는 명령어의 위험도를 AST 기반으로 분석한다.
	// 파싱 실패 시 LevelUserApproval (FAIL-CLOSED) 을 반환한다.
	// Ported from: claude_cli/src/utils/bash/ast.ts:parseForSecurity
	Analyze(ctx context.Context, command string) (Analysis, error)
}
