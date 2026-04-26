// Package tools
// File: shell_exec_direct.go
// Description: shell_exec 도구의 기본 실행 경로 및 stdin 라우팅
// Responsibility: 권한 승격이 필요 없는 직접 실행과 대화형 세션 stdin 주입

package tools

import (
	"context"
	"sync"

	"github.com/yourorg/infractl/internal/executor"
)

// executeDirect는 권한 승격 없이 명령을 실행한다.
// executor가 StreamExecutor를 지원하면 라인 단위 스트리밍으로, 아니면 일반 Execute로 폴백한다.
func (t *ShellExecTool) executeDirect(ctx context.Context, exec executor.Executor, cmd string) (executor.ExecResult, error) {
	if se, ok := exec.(executor.StreamExecutor); ok {
		session, err := se.ExecuteStream(ctx, cmd, func(line string) {
			EmitOutput(ctx, line)
		})
		if err != nil {
			return executor.ExecResult{}, err
		}
		return session.Wait()
	}
	return exec.Execute(ctx, cmd)
}

// stdinRouter는 대화형 실행 세션에 stdin 라인을 주입한다.
// 세션 바인딩 전 도착한 입력은 대기열에 두었다가 바인딩 시 재생한다.
type stdinRouter struct {
	mu      sync.Mutex
	exec    executor.StdinInjector
	session executor.ExecSession
	pending []string
}

func newStdinRouter(exec executor.Executor) *stdinRouter {
	inj, _ := exec.(executor.StdinInjector)
	return &stdinRouter{exec: inj}
}

func (r *stdinRouter) BindSession(session executor.ExecSession) error {
	r.mu.Lock()
	r.session = session
	pending := append([]string(nil), r.pending...)
	r.pending = nil
	execInj := r.exec
	r.mu.Unlock()

	for _, line := range pending {
		if execInj != nil {
			if err := execInj.InjectStdin(line); err != nil {
				return err
			}
			continue
		}
		if err := session.InjectStdin(line); err != nil {
			return err
		}
	}
	return nil
}

func (r *stdinRouter) Inject(line string) error {
	r.mu.Lock()
	execInj := r.exec
	session := r.session
	if execInj == nil && session == nil {
		r.pending = append(r.pending, line)
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if execInj != nil {
		return execInj.InjectStdin(line)
	}
	return session.InjectStdin(line)
}
