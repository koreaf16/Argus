// Package context
// File: env_info.go
// Description: 실행 환경 정보(cwd, platform, ISO date, git branch) 수집
// Responsibility: 도구 실행 시점 환경 메타데이터를 키-값 맵으로 반환

package context

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// EnvInfo 는 현재 실행 환경 정보를 담는다.
type EnvInfo struct {
	CWD       string
	Platform  string
	Date      string // ISO 8601 YYYY-MM-DD
	GitBranch string // 빈 문자열이면 git 없음
}

// GetEnvInfo 는 현재 환경 정보를 수집하여 반환한다.
func GetEnvInfo(ctx context.Context) EnvInfo {
	cwd, _ := os.Getwd()
	return EnvInfo{
		CWD:       cwd,
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Date:      time.Now().UTC().Format("2006-01-02"),
		GitBranch: gitCurrentBranch(ctx),
	}
}

func gitCurrentBranch(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
