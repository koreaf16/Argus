// Package shell — shell provider abstraction and common types.
//
// 파일 역할: ShellType 상수, ExecCommandOpts/Result 타입, ShellProvider 인터페이스 정의.
//
//	bash/powershell 두 셸 구현체가 공통으로 구현하는 계약을 명시한다.
//
// 포함 모듈:
//   - ShellType: 셸 종류를 나타내는 string alias
//   - ExecCommandOpts: BuildExecCommand 호출 옵션 (ID, 샌드박스 여부 등)
//   - ExecCommandResult: 빌드된 명령 문자열 + cwd 추적 파일 경로
//   - ShellProvider: bash/powershell 구현체가 구현할 인터페이스
//
// 호출/사용 방식:
//   - internal/tools/bash 및 internal/tools/powershell 에서 구현체를 생성해 사용
//   - CreateBashShellProvider / CreatePowerShellProvider 가 이 인터페이스를 반환
//
// 연결:
//   - 원본: src/utils/shell/shellProvider.ts
package shell

import "context"

// ShellType 은 지원하는 셸 종류를 나타낸다.
type ShellType string

const (
	ShellTypeBash    ShellType = "bash"
	ShellTypePS      ShellType = "powershell"
	DefaultHookShell ShellType = ShellTypeBash
)

// ExecCommandOpts 는 BuildExecCommand 에 전달되는 옵션이다.
type ExecCommandOpts struct {
	ID            string
	SandboxTmpDir string
	UseSandbox    bool
}

// ExecCommandResult 는 BuildExecCommand 가 반환하는 결과다.
type ExecCommandResult struct {
	// CommandString 은 spawn 시 셸에 전달할 완성된 명령 문자열이다.
	CommandString string
	// CwdFilePath 는 명령 실행 후 현재 디렉토리가 기록될 파일의 경로다.
	CwdFilePath string
}

// ShellProvider 는 bash/powershell 구현체가 공통으로 구현해야 할 인터페이스다.
type ShellProvider interface {
	// ProviderType 은 이 구현체의 셸 종류를 반환한다.
	ProviderType() ShellType

	// ShellPath 는 셸 바이너리의 경로를 반환한다.
	ShellPath() string

	// IsDetached 는 프로세스 detach 여부를 반환한다 (bash: true, powershell: false).
	IsDetached() bool

	// BuildExecCommand 는 셸 실행에 필요한 완성된 명령 문자열과 cwd 파일 경로를 반환한다.
	BuildExecCommand(ctx context.Context, command string, opts ExecCommandOpts) (ExecCommandResult, error)

	// GetSpawnArgs 는 셸 바이너리에 전달할 인수 목록을 반환한다.
	GetSpawnArgs(commandString string) []string

	// GetEnvironmentOverrides 는 이 셸 전용 환경 변수 오버라이드 맵을 반환한다.
	GetEnvironmentOverrides(ctx context.Context, command string) (map[string]string, error)
}
