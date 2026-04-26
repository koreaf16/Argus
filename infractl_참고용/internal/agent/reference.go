// Package agent
// File: reference.go
// Description: 대규모 텍스트 플레이스홀더(Reference) 관리
// Responsibility: 텍스트를 로컬 캐시에 저장하고 플레이스홀더 반환, 필요 시 플레이스홀더 확장

package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/serverenv"
)

var (
	// placeholderRegex matches [Command Output #<hash>: +<lines> lines]
	placeholderRegex = regexp.MustCompile(`\[Command Output #([a-f0-9]+): \+\d+ lines\]`)
)

func getCacheDir() string {
	stateDir, err := serverenv.StateDir()
	if err != nil {
		return os.TempDir()
	}
	cacheDir := filepath.Join(stateDir, "cache")
	os.MkdirAll(cacheDir, 0755)
	return cacheDir
}

// SaveLargeOutput 저장소에 원본 출력을 기록하고 플레이스홀더 문자열을 반환한다.
func SaveLargeOutput(content string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))[:8]
	lines := strings.Count(content, "\n")
	if lines == 0 && len(content) > 0 {
		lines = 1
	}

	cacheFile := filepath.Join(getCacheDir(), hash+".txt")
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		_ = os.WriteFile(cacheFile, []byte(content), 0644)
	}

	return fmt.Sprintf("[Command Output #%s: +%d lines]", hash, lines)
}

// ExpandPlaceholders 텍스트 내의 플레이스홀더를 감지하여 원본 내용으로 치환한다.
func ExpandPlaceholders(text string) string {
	return placeholderRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatches := placeholderRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		hash := submatches[1]
		cacheFile := filepath.Join(getCacheDir(), hash+".txt")
		content, err := os.ReadFile(cacheFile)
		if err == nil {
			return string(content)
		}
		return match
	})
}
