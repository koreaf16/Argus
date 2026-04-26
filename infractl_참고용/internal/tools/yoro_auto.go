package tools

import "strings"

var yoroProceedTokens = []string{
	"yes", "continue", "proceed", "go ahead", "start", "run", "execute", "apply",
	"default", "recommended", "approve", "allow",
	"예", "진행", "계속", "시작", "실행", "적용", "기본", "권장", "추천", "승인", "허용",
}

var yoroAvoidTokens = []string{
	"no", "cancel", "abort", "stop", "skip", "later", "manual", "deny", "reject",
	"아니오", "취소", "중단", "정지", "건너", "나중", "수동", "보류", "거부",
}

// AutoQuestionResponse picks the most proceed-oriented answer for YORO mode.
func AutoQuestionResponse(req QuestionRequest) QuestionResponse {
	if len(req.Options) == 0 {
		return QuestionResponse{
			SelectedIndex: -1,
			UserInput:     "continue",
			IsOther:       true,
		}
	}

	bestIdx := 0
	bestScore := autoQuestionScore(req.Options[0])
	for i := 1; i < len(req.Options); i++ {
		score := autoQuestionScore(req.Options[i])
		if score > bestScore {
			bestIdx = i
			bestScore = score
		}
	}

	return QuestionResponse{
		SelectedIndex: bestIdx,
		SelectedLabel: req.Options[bestIdx].Label,
		UserInput:     req.Options[bestIdx].Label,
	}
}

// AutoFormResponse fills defaults for YORO mode so forms do not block execution.
func AutoFormResponse(req FormRequest) FormResponse {
	values := make(map[string]string, len(req.Fields))
	for _, field := range req.Fields {
		values[field.Name] = autoFormValue(field)
	}
	return FormResponse{Values: values}
}

func autoQuestionScore(opt QuestionOption) int {
	text := strings.ToLower(strings.TrimSpace(opt.Label + " " + opt.Description))
	score := 0
	for _, token := range yoroProceedTokens {
		if strings.Contains(text, token) {
			score += 10
		}
	}
	for _, token := range yoroAvoidTokens {
		if strings.Contains(text, token) {
			score -= 10
		}
	}
	return score
}

func autoFormValue(field FormFieldDef) string {
	if strings.TrimSpace(field.DefaultValue) != "" {
		return field.DefaultValue
	}
	if len(field.Options) > 0 {
		return field.Options[0]
	}
	return ""
}
