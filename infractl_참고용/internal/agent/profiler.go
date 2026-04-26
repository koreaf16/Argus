// Package agent
// File: profiler.go
// Description: 서버 접속 시 OS 및 환경을 스캔하는 프로파일러 모듈
// Responsibility: 원격 서버의 OS 식별, 주요 설치 애플리케이션 감지 및 프로파일 생성

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// Profiler는 서버 환경을 스캔하고 식별하는 역할을 담당한다.
type Profiler struct {
	manager *executor.Manager
}

// NewProfiler는 새 Profiler를 생성한다.
func NewProfiler(mgr *executor.Manager) *Profiler {
	return &Profiler{
		manager: mgr,
	}
}

// Profile은 대상 서버를 스캔하고 store.Server 구조체에 OS 및 EnvProfile을 채워 반환한다.
func (p *Profiler) Profile(ctx context.Context, server store.Server) (store.Server, error) {
	exec, err := p.manager.Get(server.Name)
	if err != nil {
		return server, fmt.Errorf("get executor for %s: %w", server.Name, err)
	}

	// 1. OS 스캔
	osInfo, err := p.scanOS(ctx, exec)
	if err != nil {
		slog.Warn("failed to scan OS", "server", server.Name, "err", err)
		osInfo = "Unknown"
	}
	server.OS = osInfo

	// 2. 주요 환경 스캔 (Docker, DB 등)
	envInfo, err := p.scanEnv(ctx, exec)
	if err != nil {
		slog.Warn("failed to scan env", "server", server.Name, "err", err)
	} else {
		server.EnvProfile = envInfo
	}

	return server, nil
}

// scanOS는 원격 서버의 OS 정보를 확인한다.
func (p *Profiler) scanOS(ctx context.Context, exec executor.Executor) (string, error) {
	// 리눅스 계열 확인 시도 (os-release)
	res, err := exec.Execute(ctx, "cat /etc/os-release | grep -E '^PRETTY_NAME=' | cut -d'=' -f2 | tr -d '\"'")
	if err == nil && strings.TrimSpace(res.Stdout) != "" {
		return strings.TrimSpace(res.Stdout), nil
	}

	// Windows 계열 확인 시도 (PowerShell 또는 cmd)
	// cmd.exe /c ver 와 달리 Windows에서는 systeminfo나 ver를 사용할 수 있다.
	res, err = exec.Execute(ctx, "ver")
	if err == nil {
		out := strings.TrimSpace(res.Stdout)
		if strings.Contains(strings.ToLower(out), "windows") {
			return out, nil
		}
	}

	// 기본 fall-back (uname)
	res, err = exec.Execute(ctx, "uname -srm")
	if err == nil && strings.TrimSpace(res.Stdout) != "" {
		return strings.TrimSpace(res.Stdout), nil
	}

	return "Unknown OS", nil
}

// scanEnv는 주요 도구(Docker_ etc) 설치 여부를 스캔한다.
func (p *Profiler) scanEnv(ctx context.Context, exec executor.Executor) (string, error) {
	var envs []string

	// Docker 확인
	if res, err := exec.Execute(ctx, "docker --version"); err == nil && res.ExitCode == 0 {
		envs = append(envs, "Docker")
	}

	// Kubernetes (kubectl) 확인
	if res, err := exec.Execute(ctx, "kubectl version --client --short"); err == nil && res.ExitCode == 0 {
		envs = append(envs, "Kubernetes")
	}

	// Python 확인
	if res, err := exec.Execute(ctx, "python3 --version"); err == nil && res.ExitCode == 0 {
		envs = append(envs, "Python3")
	}

	return strings.Join(envs, ", "), nil
}
