// Package utils — 서브프로세스 실행 및 ShellCommand 추상화.
package utils

import (
	"io"
)

// ShellCommandStatus 는 실행 중인 명령의 생명주기 상태를 나타낸다.
type ShellCommandStatus string

const (
	ShellStatusRunning      ShellCommandStatus = "running"
	ShellStatusBackgrounded ShellCommandStatus = "backgrounded"
	ShellStatusCompleted    ShellCommandStatus = "completed"
	ShellStatusKilled       ShellCommandStatus = "killed"
)

// ShellCommand 는 실행 중인 자식 프로세스를 감싼다.
type ShellCommand struct {
	// Result 는 명령이 완료되면 ExecResult 를 한 번 전송한다.
	Result <-chan ExecResult

	// Stream 은 명령 실행 중 발생하는 실시간 출력을 전송한다.
	Stream chan string

	// Stdin 은 프로세스의 표준 입력에 쓸 수 있는 writer 다.
	Stdin io.Writer

	// Kill 은 프로세스를 강제 종료한다.
	Kill func()

	// Status 는 현재 명령 상태다.
	Status ShellCommandStatus

	// Cleanup 은 스트림 리소스를 해제한다.
	Cleanup func()

	// Background 는 명령을 백그라운드 태스크로 전환한다.
	Background func(taskID string) bool

	// isPTY 는 PTY 시뮬레이션 모드 사용 여부를 나타낸다.
	isPTY bool
}

// Write 는 프로세스의 stdin 에 데이터를 쓴다.
func (sc *ShellCommand) Write(data string) error {
	if sc.Stdin == nil {
		return io.EOF
	}
	_, err := sc.Stdin.Write([]byte(data))
	return err
}

// CreateFailedCommand 는 즉시 오류 결과를 전송하는 ShellCommand 를 반환한다.
func CreateFailedCommand(err error) *ShellCommand {
	ch := make(chan ExecResult, 1)
	ch <- ExecResult{
		Code:          1,
		Stderr:        err.Error(),
		PreSpawnError: err.Error(),
	}
	return &ShellCommand{
		Result:     ch,
		Kill:       func() {},
		Status:     ShellStatusKilled,
		Cleanup:    func() {},
		Background: func(string) bool { return false },
	}
}

// CreateAbortedCommand 는 사용자 중단(Interrupted=true)을 전송하는 ShellCommand 를 반환한다.
func CreateAbortedCommand() *ShellCommand {
	ch := make(chan ExecResult, 1)
	ch <- ExecResult{
		Code:        1,
		Interrupted: true,
	}
	return &ShellCommand{
		Result:     ch,
		Kill:       func() {},
		Status:     ShellStatusKilled,
		Cleanup:    func() {},
		Background: func(string) bool { return false },
	}
}

// RunCommand 인터페이스 선언 (실제 구현은 플랫폼별 파일에서 담당)
// RunCommand(ctx context.Context, name string, args []string, env map[string]string, cwd string, timeout time.Duration, usePTY bool) *ShellCommand
