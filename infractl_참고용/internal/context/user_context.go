// Package context
// File: user_context.go
// Description: 사용자 정의 컨텍스트 파일(INFRACTL.md) 로드 래퍼
// Responsibility: current workspace .infractl/INFRACTL.md content loader

package context

import (
	"os"
	"path/filepath"

	"github.com/yourorg/infractl/internal/serverenv"
)

// LoadUserContext reads .infractl/INFRACTL.md from the current workspace.
// 파일이 없으면 빈 문자열을 반환한다.
func LoadUserContext() string {
	stateDir, err := serverenv.StateDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(stateDir, "INFRACTL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
