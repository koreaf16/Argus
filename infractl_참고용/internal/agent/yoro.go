package agent

import (
	"fmt"
	"regexp"

	"github.com/yourorg/infractl/internal/llm"
)

const maxYOROAutoContinueRetries = 3

var yoroConfirmationPattern = regexp.MustCompile(
	`(?i)(진행하시겠습니까|진행할까요|시작하시겠습니까|시작할까요|계속하시겠습니까|계속할까요|실행할까요|실행하시겠습니까|원하십니까|어떻게 진행|진행하겠습니다|계속하겠습니다|시작하겠습니다|진행합니다|shall i|should i proceed|would you like|do you want me to|can i go ahead|i will proceed|i will continue|i will start)`,
)

func isYOROConfirmationResponse(content string) bool {
	return yoroConfirmationPattern.MatchString(content)
}

func yoroAutoContinueMessage(attempt int) llm.Message {
	return llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(
			"[YORO AUTO-CONTINUE %d] YORO mode is active. Do not ask for confirmation. Skip the approval step and continue with the next concrete action immediately.",
			attempt,
		),
	}
}
