// Package shell
// File: prepared.go
// Description: ShellProvider.Prepare 결과 타입 정의
// Responsibility: exec.Command 에 전달 가능한 완성 argv + 분석 결과 + 정리 함수 묶음

package shell

// PreparedCmd 는 ShellProvider.Prepare 의 반환값이다.
// Executor 는 Argv[0] 을 바이너리, Argv[1:] 을 인자로 그대로 exec.Command 에 넘긴다.
type PreparedCmd struct {
	// Argv 는 exec.Command 용 완성 인자 목록.
	// 예: ["bash", "-c", "set -f; ..."] 또는 ["powershell.exe", "-NoProfile", "-EncodedCommand", "<b64>"]
	Argv []string
	// Analysis 는 Prepare 와 동시에 수행된 위험 분석 결과다.
	Analysis Analysis
	// CleanupFns 는 명령 실행 완료 후 호출할 정리 함수 목록이다.
	// snapshot 임시 파일 삭제, PowerShell exitcode 파일 삭제 등에 사용한다.
	// Executor 는 Execute 반환 직전에 defer 로 순서대로 호출해야 한다.
	CleanupFns []func() error
	// UsesPTY 는 PTY 기반 실행이 권장되는지 여부를 힌트로 제공한다.
	// 실제 PTY 생성 여부는 Executor 가 결정한다.
	UsesPTY bool
}
