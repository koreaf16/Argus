package agent

import "strings"

// DetectIdleInputHint returns a short user-facing hint when a shell command is
// likely blocked on stdin for a known reason.
func DetectIdleInputHint(command string) string {
	if strings.Contains(strings.ToLower(command), "<<") {
		return "힌트: 이 명령은 here-doc(`<<EOF`) 종료를 기다리는 중일 수 있습니다. 중단 후 `printf` 같은 비대화형 방식으로 다시 실행하는 편이 안전합니다."
	}
	return ""
}
