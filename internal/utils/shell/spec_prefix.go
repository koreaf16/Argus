// Package shell — fig-spec 기반 명령 프리픽스 추출 (순수 문자열 로직).
//
// 파일 역할: 알려진 CLI 명령의 깊이 규칙(DepthRules)을 정의하고,
//
//	명령 + 인수 배열에서 허가 규칙 매칭용 프리픽스를 빌드한다.
//	LLM/Haiku 의존성 없는 순수 함수다.
//
// 포함 모듈:
//   - DepthRules: 명령별 최대 프리픽스 깊이 맵
//   - BuildPrefix: 명령과 인수에서 프리픽스 문자열 생성
//
// 호출/사용 방식:
//   - internal/tools/bash 에서 허가 검사 시 프리픽스를 결정할 때 호출
//   - internal/tools/powershell 에서도 동일하게 사용
//
// 연결:
//   - 원본: src/utils/shell/specPrefix.ts
package shell

import "strings"

// DepthRules 는 알려진 명령의 최대 프리픽스 깊이를 정의한다.
// e.g. "git push" → 2 는 "git push <remote>" 에서 <remote> 까지만 포함함을 의미한다.
var DepthRules = map[string]int{
	"rg":             2,
	"pre-commit":     2,
	"gcloud":         4,
	"gcloud compute": 6,
	"gcloud beta":    6,
	"aws":            4,
	"az":             4,
	"kubectl":        3,
	"docker":         3,
	"dotnet":         3,
	"git push":       2,
}

// BuildPrefix 는 명령 이름과 인수 목록에서 허가 규칙 매칭용 프리픽스를 반환한다.
//
// Phase 1 단순화 알고리즘:
//  1. DepthRules 에서 (command + 첫 번째 비플래그 인수) 또는 command 의 깊이를 조회한다.
//  2. 플래그(-로 시작)는 건너뛰고, 비플래그 토큰을 깊이 한도까지 포함한다.
//
// 예: BuildPrefix("git", ["-C", "/repo", "status"]) → "git status"
func BuildPrefix(command string, args []string) string {
	// 첫 번째 비플래그 인수를 찾아 복합 키 조회
	firstPositional := ""
	for _, a := range args {
		if a != "" && !strings.HasPrefix(a, "-") {
			firstPositional = strings.ToLower(a)
			break
		}
	}

	commandLower := strings.ToLower(command)
	maxDepth := 2 // 기본 깊이
	if firstPositional != "" {
		if d, ok := DepthRules[commandLower+" "+firstPositional]; ok {
			maxDepth = d
		} else if d, ok := DepthRules[commandLower]; ok {
			maxDepth = d
		}
	} else if d, ok := DepthRules[commandLower]; ok {
		maxDepth = d
	}

	parts := []string{command}
	for _, arg := range args {
		if len(parts) >= maxDepth {
			break
		}
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		parts = append(parts, arg)
	}

	return strings.Join(parts, " ")
}
