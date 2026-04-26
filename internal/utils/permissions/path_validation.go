// Package permissions — 파일 경로 권한 검증.
//
// 파일 역할: 파일 읽기·쓰기·생성 작업 전 경로 안전성을 검사한다.
//
//	작업 디렉토리 범위 확인, glob 기반 디렉토리 추출, 틸드 확장 등을 포함한다.
//
// 포함 모듈:
//   - FileOperationType: 파일 작업 종류 (read/write/create).
//   - PathCheckResult / ResolvedPathCheckResult: 경로 검사 결과.
//   - ValidatePath(): 경로 허용 여부 확인.
//   - ExpandTilde(): 틸드를 홈 디렉토리로 확장.
//   - GetGlobBaseDirectory(): glob 패턴에서 베이스 디렉토리 추출.
//
// 호출/사용 방식:
//   - internal/tools/bash/path_validation.go 에서 파일 작업 전 검사 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/pathValidation.ts
package permissions

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/koreaf16/argus/internal/types"
)

// FileOperationType 은 파일 작업 종류다.
type FileOperationType string

const (
	FileOpRead   FileOperationType = "read"
	FileOpWrite  FileOperationType = "write"
	FileOpCreate FileOperationType = "create"
)

// PathCheckResult 는 경로 검사 결과다.
type PathCheckResult struct {
	Allowed        bool
	DecisionReason *types.PermissionDecisionReason
}

// ResolvedPathCheckResult 는 확인된 경로를 포함한 경로 검사 결과다.
type ResolvedPathCheckResult struct {
	PathCheckResult
	ResolvedPath string
}

var globPatternRe = regexp.MustCompile(`[*?\[\]{}]`)

// GetGlobBaseDirectory 는 glob 패턴에서 베이스 디렉토리를 추출한다.
// 예: "/path/to/*.txt" → "/path/to"
func GetGlobBaseDirectory(path string) string {
	loc := globPatternRe.FindStringIndex(path)
	if loc == nil {
		return path
	}
	beforeGlob := path[:loc[0]]
	sep := "/"
	if runtime.GOOS == "windows" {
		// Windows: 슬래시와 역슬래시 모두 처리
		lastFwd := strings.LastIndex(beforeGlob, "/")
		lastBack := strings.LastIndex(beforeGlob, `\`)
		idx := lastFwd
		if lastBack > idx {
			idx = lastBack
		}
		if idx == -1 {
			return "."
		}
		return beforeGlob[:idx]
	}
	idx := strings.LastIndex(beforeGlob, sep)
	if idx == -1 {
		return "."
	}
	if idx == 0 {
		return "/"
	}
	return beforeGlob[:idx]
}

// ExpandTilde 는 경로 앞의 '~' 를 홈 디렉토리로 치환한다.
// ~username 확장은 보안상 지원하지 않는다.
func ExpandTilde(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(path, `~\`)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// FormatDirectoryList 는 디렉토리 목록을 사용자용 문자열로 포맷한다.
func FormatDirectoryList(dirs []string) string {
	const max = 5
	quoted := make([]string, len(dirs))
	for i, d := range dirs {
		quoted[i] = "'" + d + "'"
	}
	if len(dirs) <= max {
		return strings.Join(quoted, ", ")
	}
	first := strings.Join(quoted[:max], ", ")
	return first + ", 외 " + string(rune('0'+len(dirs)-max)) + "개"
}

// IsPathInWorkingDir 는 path 가 workingDir 하위에 있는지 확인한다.
func IsPathInWorkingDir(path, workingDir string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absWD, err := filepath.Abs(workingDir)
	if err != nil {
		return false
	}
	// Rel 이 성공하고 ".." 으로 시작하지 않으면 하위 경로
	rel, err := filepath.Rel(absWD, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// ValidatePath 는 경로가 현재 작업 디렉토리 범위 내에 있는지 확인한다.
// 읽기 작업은 홈 디렉토리 외부를 허용하지 않는다(Phase 1 간소화).
func ValidatePath(
	path string,
	opType FileOperationType,
	cwd string,
	permCtx types.ToolPermissionContext,
) PathCheckResult {
	// 틸드 확장
	path = ExpandTilde(path)

	// glob 패턴이면 베이스 디렉토리를 사용
	basePath := GetGlobBaseDirectory(path)

	// 현재 작업 디렉토리 범위 확인
	if !IsPathInWorkingDir(basePath, cwd) {
		// 추가 허용 디렉토리 확인
		for _, awd := range permCtx.AdditionalWorkingDirectories {
			if IsPathInWorkingDir(basePath, awd.Path) {
				return PathCheckResult{Allowed: true}
			}
		}
		reason := &types.PermissionDecisionReason{
			Type:   types.DecisionReasonWorkingDir,
			Reason: "경로가 현재 작업 디렉토리 범위를 벗어났습니다: " + path,
		}
		return PathCheckResult{Allowed: false, DecisionReason: reason}
	}

	return PathCheckResult{Allowed: true}
}
