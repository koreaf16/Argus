package hooks

import (
	"encoding/json"
	"strings"

	"github.com/koreaf16/argus/internal/types"
	permutils "github.com/koreaf16/argus/internal/utils/permissions"
)

// matchesTool 은 훅 매처가 주어진 도구 이름과 매칭되는지 반환한다.
// 빈 문자열 matcher 는 모든 도구에 매칭된다.
func matchesTool(matcher, toolName string) bool {
	if matcher == "" {
		return true
	}
	return strings.EqualFold(matcher, toolName)
}

// matchesIfCondition 은 훅의 if 조건이 현재 도구 호출과 매칭되는지 판단한다.
// if 조건은 "Bash(git *)" 형식의 permission rule 구문을 사용한다.
func matchesIfCondition(ifCond string, input HookInput) bool {
	if ifCond == "" {
		return true
	}
	// 비도구 이벤트(SessionStart 등)는 if 조건 평가 불가 → 통과
	if input.ToolName == "" {
		return true
	}

	rv := permutils.PermissionRuleValueFromString(ifCond)
	if !strings.EqualFold(rv.ToolName, input.ToolName) {
		return false
	}
	if rv.RuleContent == "" {
		return true
	}

	// tool input JSON에서 "command" 필드 추출
	var inputMap map[string]json.RawMessage
	command := ""
	if err := json.Unmarshal([]byte(input.ToolInputJSON), &inputMap); err == nil {
		if raw, ok := inputMap["command"]; ok {
			_ = json.Unmarshal(raw, &command)
		}
	}

	rule := permutils.ParsePermissionRule(rv.RuleContent)
	caseInsensitive := strings.EqualFold(input.ToolName, "powershell")
	return permutils.MatchesPermissionRule(rule, command, caseInsensitive)
}

// resolveCommands 는 HooksConfig에서 이벤트와 도구 이름에 매칭되는
// 훅 명령 목록을 등록 순서대로 반환한다.
func resolveCommands(cfg HooksConfig, event types.HookEvent, toolName string) []HookCommand {
	var result []HookCommand
	matchers, ok := cfg[event]
	if !ok {
		return nil
	}
	for _, m := range matchers {
		if matchesTool(m.Matcher, toolName) {
			result = append(result, m.Hooks...)
		}
	}
	return result
}
