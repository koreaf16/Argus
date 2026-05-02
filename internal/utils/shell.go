// Package utils — 최상위 유틸리티: 셸 탐지·실행 결과 타입·설정.
//
// 파일 역할: 셸 실행 결과를 담는 ExecResult 타입과 ShellConfig 타입을 정의한다.
//
//	FindSuitableShell 함수로 시스템에서 사용 가능한 bash/zsh 를 탐색한다.
//
// 포함 모듈:
//   - ExecResult: 셸 명령 실행 결과 (stdout, stderr, 코드, 백그라운드 정보 등)
//   - ShellConfig: 셸 프로바이더를 보유하는 세션 설정 컨테이너
//   - FindSuitableShell: CLAUDE_CODE_SHELL 환경변수 → bash/zsh → Git Bash 순서로 탐색
//
// 호출/사용 방식:
//   - internal/tools/bash 패키지의 BashTool 에서 셸 실행 시 사용
//   - internal/query/engine.go 에서 ExecResult 를 도구 응답으로 소비
//
// 연결:
//   - 원본: src/utils/Shell.ts
//   - import: internal/utils/shell (ShellProvider 인터페이스)
package utils

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/koreaf16/argus/internal/utils/shell"
)

// ExecResult 는 셸 명령 실행 결과를 담는다.
type ExecResult struct {
	Stdout                    string `json:"stdout"`
	Stderr                    string `json:"stderr"`
	Code                      int    `json:"code"`
	Interrupted               bool   `json:"interrupted"`
	ActiveLane                string `json:"active_lane"`
	ActiveUser                string `json:"active_user"`
	ActiveCWD                 string `json:"active_cwd"`
	BackgroundTaskID          string `json:"background_task_id,omitempty"`
	OutputFilePath            string `json:"output_file_path,omitempty"`
	OutputFileSize            int64  `json:"output_file_size,omitempty"`
	OutputTaskID              string `json:"output_task_id,omitempty"`
	PreSpawnError             string `json:"pre_spawn_error,omitempty"`
	AssistantAutoBackgrounded bool   `json:"assistant_auto_backgrounded"`
	BackgroundedByUser        bool   `json:"backgrounded_by_user"`
}

// ShellConfig 는 세션에서 사용할 셸 프로바이더를 보유한다.
type ShellConfig struct {
	Provider shell.ShellProvider
}

// FindSuitableShell 은 실행 가능한 셸 경로를 반환한다.
// 우선순위: CLAUDE_CODE_SHELL 환경변수 → SHELL 환경변수 (bash/zsh) → PATH 탐색 → Windows Git Bash.
func FindSuitableShell() (string, error) {
	// 1. 명시적 오버라이드
	if override := os.Getenv("CLAUDE_CODE_SHELL"); override != "" {
		lower := strings.ToLower(override)
		if strings.Contains(lower, "bash") || strings.Contains(lower, "zsh") {
			if isExecutable(override) {
				return override, nil
			}
		}
	}

	// 2. SHELL 환경변수 (Unix/WSL)
	if envShell := os.Getenv("SHELL"); envShell != "" {
		lower := strings.ToLower(envShell)
		if (strings.Contains(lower, "bash") || strings.Contains(lower, "zsh")) && isExecutable(envShell) {
			return envShell, nil
		}
	}

	// 3. PATH 탐색
	if p, err := exec.LookPath("bash"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("zsh"); err == nil {
		return p, nil
	}

	// 4. Windows: Git Bash
	if runtime.GOOS == "windows" {
		candidates := []string{
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files (x86)\Git\bin\bash.exe`,
		}
		for _, c := range candidates {
			if isExecutable(c) {
				return c, nil
			}
		}
		// Git Bash on PATH (msys / cygwin 환경)
		return "bash", nil
	}

	return "", nil
}

// isExecutable 은 파일이 실행 가능한지 빠르게 검사한다.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	// Windows 에서는 확장자로 판단
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(path)
		return strings.HasSuffix(lower, ".exe") ||
			strings.HasSuffix(lower, ".bat") ||
			strings.HasSuffix(lower, ".cmd")
	}
	// Unix: 실행 비트 확인
	return info.Mode()&0o111 != 0
}
