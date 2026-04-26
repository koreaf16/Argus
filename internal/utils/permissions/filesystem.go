// Package permissions — 파일 시스템 권한 검사.
//
// 파일 역할: 파일 읽기·쓰기·생성 작업의 권한을 확인한다.
//
//	위험한 경로(설정 파일, .git 등) 보호, 내부 허용 경로 관리,
//	규칙 기반 파일 접근 제어를 구현한다.
//
// 포함 모듈:
//   - DangerousFiles / DangerousDirectories: 보호 대상 경로 목록.
//   - IsDangerousFilePath(): 위험 경로 여부 판단.
//   - CheckWritePermissionForTool() / CheckReadPermissionForTool(): 도구별 권한 검사.
//   - GetArgusConfigDir() / GetSessionMemoryPath(): 내부 경로 헬퍼.
//
// 호출/사용 방식:
//   - internal/tools/bash/permissions.go 에서 파일 작업 전 검사 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/filesystem.ts (핵심 로직만 포함)
package permissions

import (
	"path/filepath"
	"strings"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/types"
)

// DangerousFiles 는 자동 편집으로부터 보호해야 할 설정 파일 이름 목록이다.
var DangerousFiles = []string{
	".gitconfig",
	".bashrc", ".bash_profile", ".bash_login",
	".zshrc", ".zprofile",
	".profile",
	".ssh/config", ".ssh/authorized_keys", ".ssh/known_hosts",
	".npmrc", ".pypirc",
	".netrc", ".curlrc",
	".aws/credentials",
}

// DangerousDirectories 는 자동 편집으로부터 보호해야 할 디렉토리 이름 목록이다.
var DangerousDirectories = []string{
	".git",
	".vscode",
	".idea",
	".ssh",
	".gnupg",
	".argus", // 자기 설정 보호
}

// GetArgusConfigDir 는 ~/.argus/ 경로를 반환한다.
func GetArgusConfigDir() string {
	return constants.ConfigDir()
}

// GetSessionMemoryPath 는 세션 메모리 파일 경로를 반환한다.
func GetSessionMemoryPath(sessionID string) string {
	return filepath.Join(constants.ConfigDir(), "sessions", sessionID+".json")
}

// IsDangerousFilePath 는 자동 편집이 위험할 수 있는 경로인지 판단한다.
func IsDangerousFilePath(path string) (bool, string) {
	normalizedPath := filepath.ToSlash(strings.ToLower(path))

	for _, dangerFile := range DangerousFiles {
		if strings.HasSuffix(normalizedPath, "/"+strings.ToLower(dangerFile)) ||
			strings.ToLower(filepath.Base(path)) == strings.ToLower(dangerFile) {
			return true, "위험한 설정 파일입니다: " + dangerFile
		}
	}

	for _, dangerDir := range DangerousDirectories {
		if strings.Contains(normalizedPath, "/"+strings.ToLower(dangerDir)+"/") ||
			strings.HasSuffix(normalizedPath, "/"+strings.ToLower(dangerDir)) {
			return true, "위험한 디렉토리입니다: " + dangerDir
		}
	}

	return false, ""
}

// CheckWritePermissionForTool 는 파일 쓰기·생성 작업의 권한을 확인한다.
func CheckWritePermissionForTool(
	path string,
	cwd string,
	permCtx types.ToolPermissionContext,
) PathCheckResult {
	// 위험 경로 확인
	if dangerous, reason := IsDangerousFilePath(path); dangerous {
		dr := &types.PermissionDecisionReason{
			Type:                 types.DecisionReasonSafetyCheck,
			Reason:               reason,
			ClassifierApprovable: false,
		}
		return PathCheckResult{Allowed: false, DecisionReason: dr}
	}

	// 경로 범위 확인
	return ValidatePath(path, FileOpWrite, cwd, permCtx)
}

// CheckReadPermissionForTool 는 파일 읽기 작업의 권한을 확인한다.
// 읽기는 쓰기보다 관대하게 처리한다.
func CheckReadPermissionForTool(
	path string,
	cwd string,
	permCtx types.ToolPermissionContext,
) PathCheckResult {
	return ValidatePath(path, FileOpRead, cwd, permCtx)
}

// MatchingRuleForInput 은 입력 경로에 매칭되는 권한 규칙을 반환한다.
func MatchingRuleForInput(
	path string,
	toolName string,
	rules []types.PermissionRule,
) *types.PermissionRule {
	for _, r := range rules {
		if r.RuleValue.ToolName != toolName {
			continue
		}
		if r.RuleValue.RuleContent == "" {
			r := r
			return &r
		}
		parsed := ParsePermissionRule(r.RuleValue.RuleContent)
		if MatchesPermissionRule(parsed, path, false) {
			r := r
			return &r
		}
	}
	return nil
}
