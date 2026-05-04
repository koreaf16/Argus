package askuser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tools"
)

type AskUserTool struct{}

func NewAskUserTool() *AskUserTool {
	return &AskUserTool{}
}

// NewAskUserQuestionTool is kept for compatibility with older wiring.
func NewAskUserQuestionTool() *AskUserTool {
	return NewAskUserTool()
}

func (t *AskUserTool) Name() string {
	return "ask_user"
}

func (t *AskUserTool) Description(ctx tools.Context) string {
	return "사용자에게 하나 이상의 대화형 질문을 하고 답변을 반환합니다."
}

func (t *AskUserTool) InputSchema() tools.ToolInputJSONSchema {
	questionSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":           map[string]any{"type": "string", "description": "질문 식별 ID"},
			"header":       map[string]any{"type": "string", "description": "질문 상단에 표시될 헤더 텍스트"},
			"question":     map[string]any{"type": "string", "description": "사용자에게 물어볼 질문 내용"},
			"type":         map[string]any{"type": "string", "enum": []string{"text", "choice", "yesno"}, "description": "질문 유형 (text, choice, yesno)"},
			"placeholder":  map[string]any{"type": "string", "description": "입력 필드의 힌트 텍스트"},
			"default":      map[string]any{"type": "string", "description": "기본값"},
			"multi_select": map[string]any{"type": "boolean", "description": "다중 선택 가능 여부 (choice 유형 전용)"},
			"required":     map[string]any{"type": "boolean", "description": "필수 답변 여부"},
			"preview":      map[string]any{"type": "string", "description": "미리보기 텍스트"},
			"options": map[string]any{
				"type": "array",
				"description": "선택 옵션 목록 (choice 유형 전용)",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value":       map[string]any{"type": "string", "description": "옵션의 실제 값"},
						"label":       map[string]any{"type": "string", "description": "사용자에게 보여줄 라벨"},
						"description": map[string]any{"type": "string", "description": "옵션에 대한 추가 설명"},
					},
				},
			},
		},
		"required": []string{"question"},
	}

	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"question":     map[string]any{"type": "string", "description": "단일 질문 시 사용할 질문 내용"},
			"header":       map[string]any{"type": "string", "description": "단일 질문 시 사용할 헤더"},
			"type":         map[string]any{"type": "string", "enum": []string{"text", "choice", "yesno"}, "description": "단일 질문 시 사용할 유형"},
			"placeholder":  map[string]any{"type": "string", "description": "단일 질문 시 사용할 힌트"},
			"default":      map[string]any{"type": "string", "description": "단일 질문 시 사용할 기본값"},
			"multi_select": map[string]any{"type": "boolean", "description": "단일 질문 시 다중 선택 여부"},
			"required":     map[string]any{"type": "boolean", "description": "단일 질문 시 필수 여부"},
			"options":      questionSchema["properties"].(map[string]any)["options"],
			"questions": map[string]any{
				"type":  "array",
				"description": "여러 질문을 한 번에 할 때 사용하는 질문 목록",
				"items": questionSchema,
			},
		},
	}
}

func (t *AskUserTool) IsReadOnly() bool {
	return true
}

func (t *AskUserTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	events := make(chan tools.ToolEvent, 2)
	go func() {
		defer close(events)

		questions, err := parseQuestions(input)
		if err != nil {
			events <- tools.NewErrorEvent(err)
			return
		}

		if ctx.State != nil && ctx.State.InYoloMode() && ctx.ExecuteSubQuery != nil {
			// [YOLO] LLM 이 스스로 답변을 결정하도록 Sub-query 수행
			events <- tools.NewChunkEvent("\n[YOLO] 자율 모드: LLM 이 답변을 결정 중입니다...\n")
			batchResp, subErr := t.decideAnswersYolo(ctx, questions)
			if subErr != nil {
				events <- tools.NewErrorEvent(subErr)
				return
			}
			t.processAndEmitAnswers(ctx, events, questions, batchResp)
			return
		}

		respCh := make(chan tools.AskUserBatchResponse, 1)
		prompt := &tools.AskUserBatchPrompt{
			Questions: append([]tools.AskUserQuestion(nil), questions...),
		}
		select {
		case <-ctx.Context.Done():
			emitResult(events, map[string]string{}, map[string]string{}, nil, true)
			return
		case events <- tools.ToolEvent{
			Kind:                 tools.ToolEventAskUserBatchPrompt,
			AskUserBatchPrompt:   prompt,
			AskUserBatchResponse: respCh,
		}:
		}

		var batchResp tools.AskUserBatchResponse
		select {
		case <-ctx.Context.Done():
			emitResult(events, map[string]string{}, map[string]string{}, nil, true)
			return
		case batchResp = <-respCh:
		}
		if batchResp.Canceled {
			emitResult(events, map[string]string{}, map[string]string{}, nil, true)
			return
		}
		if strings.TrimSpace(batchResp.Error) != "" {
			events <- tools.NewErrorEvent(fmt.Errorf(strings.TrimSpace(batchResp.Error)))
			return
		}

		t.processAndEmitAnswers(ctx, events, questions, batchResp)
	}()
	return events, nil
}

func (t *AskUserTool) decideAnswersYolo(ctx tools.Context, questions []tools.AskUserQuestion) (tools.AskUserBatchResponse, error) {
	var sb strings.Builder
	sb.WriteString("You are the YOLO-mode autonomous decision maker for Argus. ")
	sb.WriteString("You must decide the most appropriate answers for the following questions on behalf of the user to ensure the task proceeds without interruption. ")
	sb.WriteString("Consider the system environment, project context, and best practices. ")
	sb.WriteString("If a default value is provided and seems reasonable, prefer it. ")
	sb.WriteString("Return your decisions as a JSON object where keys are question IDs or indices (\"0\", \"1\", ...) and values are the chosen answers.\n\n")

	for i, q := range questions {
		sb.WriteString(fmt.Sprintf("Question %d (ID: %s):\n", i, q.ID))
		sb.WriteString(fmt.Sprintf("  Text: %s\n", q.Question))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", q.Type))
		if q.Default != "" {
			sb.WriteString(fmt.Sprintf("  Default: %s\n", q.Default))
		}
		if len(q.Options) > 0 {
			sb.WriteString("  Options:\n")
			for _, opt := range q.Options {
				sb.WriteString(fmt.Sprintf("    - %s (%s): %s\n", opt.Label, opt.Value, opt.Description))
			}
		}
		sb.WriteString("\n")
	}

	systemPrompt := "You are a senior systems engineer acting as a proxy for the user. Provide concise, expert-level decisions in JSON format."
	userPrompt := sb.String()

	resp, err := ctx.ExecuteSubQuery(ctx.Context, systemPrompt, userPrompt)
	if err != nil {
		return tools.AskUserBatchResponse{}, fmt.Errorf("YOLO decision sub-query failed: %w", err)
	}

	// JSON 추출 (LLM 이 마크다운으로 감쌀 경우 대비)
	jsonStr := resp
	if start := strings.Index(jsonStr, "{"); start != -1 {
		if end := strings.LastIndex(jsonStr, "}"); end != -1 && end > start {
			jsonStr = jsonStr[start : end+1]
		}
	}

	var answers map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &answers); err != nil {
		// 파싱 실패 시 기본값이나 빈 값으로 대체 (중단 방지)
		return tools.AskUserBatchResponse{
			AnswersByIndex: make(map[string]string),
		}, nil
	}

	return tools.AskUserBatchResponse{
		AnswersByIndex: answers,
		AnswersByID:    answers,
	}, nil
}

func (t *AskUserTool) processAndEmitAnswers(ctx tools.Context, events chan<- tools.ToolEvent, questions []tools.AskUserQuestion, batchResp tools.AskUserBatchResponse) {
	answersByIndex := make(map[string]string, len(questions))
	answersByID := make(map[string]string, len(questions))
	responses := make([]map[string]string, 0, len(questions))

	for i, q := range questions {
		index := fmt.Sprintf("%d", i)
		value := ""
		if batchResp.AnswersByIndex != nil {
			value = strings.TrimSpace(batchResp.AnswersByIndex[index])
		}
		if value == "" && q.ID != "" && batchResp.AnswersByID != nil {
			value = strings.TrimSpace(batchResp.AnswersByID[q.ID])
		}
		if value == "" {
			value = strings.TrimSpace(q.Default)
		}
		if q.Type == "yesno" {
			value = normalizeYesNo(value)
		}
		// YOLO 모드이거나 기본값이 있으면 필수 체크 통과
		if q.Required && value == "" && !ctx.State.InYoloMode() {
			events <- tools.NewErrorEvent(fmt.Errorf("질문 %q에 대한 답변이 필요합니다", q.Question))
			return
		}

		answersByIndex[index] = value
		if q.ID != "" {
			answersByID[q.ID] = value
		}
		responses = append(responses, map[string]string{
			"index":    index,
			"id":       q.ID,
			"question": q.Question,
			"value":    value,
		})
	}

	if ctx.State != nil {
		ctx.State.SetWorkflowApproved(true)
	}

	emitResult(events, answersByIndex, answersByID, responses, false)
	events <- tools.NewDoneEvent()
}

func (t *AskUserTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}

func (t *AskUserTool) MaxResultSizeChars() int {
	return 10000
}

type askUserRequest struct {
	Question    string             `json:"question"`
	Header      string             `json:"header"`
	Type        string             `json:"type"`
	Placeholder string             `json:"placeholder"`
	Default     string             `json:"default"`
	MultiSelect bool               `json:"multi_select"`
	Required    *bool              `json:"required"`
	Preview     string             `json:"preview"`
	Options     []askUserOptionReq `json:"options"`
	Questions   []askUserQuestion  `json:"questions"`
}

type askUserQuestion struct {
	ID          string             `json:"id"`
	Header      string             `json:"header"`
	Question    string             `json:"question"`
	Type        string             `json:"type"`
	Placeholder string             `json:"placeholder"`
	Default     string             `json:"default"`
	MultiSelect bool               `json:"multi_select"`
	Required    *bool              `json:"required"`
	Preview     string             `json:"preview"`
	Options     []askUserOptionReq `json:"options"`
}

type askUserOptionReq struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func parseQuestions(input json.RawMessage) ([]tools.AskUserQuestion, error) {
	var req askUserRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("잘못된 ask_user 입력: %w", err)
	}

	questions := make([]askUserQuestion, 0, len(req.Questions)+1)
	questions = append(questions, req.Questions...)
	if len(questions) == 0 && strings.TrimSpace(req.Question) != "" {
		questions = append(questions, askUserQuestion{
			Header:      req.Header,
			Question:    req.Question,
			Type:        req.Type,
			Placeholder: req.Placeholder,
			Default:     req.Default,
			MultiSelect: req.MultiSelect,
			Required:    req.Required,
			Preview:     req.Preview,
			Options:     req.Options,
		})
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("ask_user는 하나 이상의 질문이 필요합니다")
	}

	out := make([]tools.AskUserQuestion, 0, len(questions))
	for i, raw := range questions {
		q, err := normalizeQuestion(i, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

func normalizeQuestion(index int, q askUserQuestion) (tools.AskUserQuestion, error) {
	text := strings.TrimSpace(q.Question)
	if text == "" {
		return tools.AskUserQuestion{}, fmt.Errorf("questions[%d].question 필드가 필요합니다", index)
	}

	questionType := strings.ToLower(strings.TrimSpace(q.Type))
	options := normalizeOptions(q.Options)
	if questionType == "" {
		if len(options) > 0 {
			questionType = "choice"
		} else {
			questionType = "text"
		}
	}

	switch questionType {
	case "text":
		options = nil
	case "choice":
		if len(options) == 0 {
			return tools.AskUserQuestion{}, fmt.Errorf("type=choice인 경우 questions[%d].options 필드가 필요합니다", index)
		}
	case "yesno":
		if len(options) == 0 {
			options = []tools.AskUserOption{
				{Value: "yes", Label: "예"},
				{Value: "no", Label: "아니요"},
			}
		}
	default:
		return tools.AskUserQuestion{}, fmt.Errorf("questions[%d].type은 text, choice, yesno 중 하나여야 합니다", index)
	}

	required := true
	if q.Required != nil {
		required = *q.Required
	}
	id := strings.TrimSpace(q.ID)
	if id == "" {
		id = fmt.Sprintf("%d", index)
	}

	return tools.AskUserQuestion{
		ID:          id,
		Header:      strings.TrimSpace(q.Header),
		Question:    text,
		Type:        questionType,
		Placeholder: strings.TrimSpace(q.Placeholder),
		Default:     strings.TrimSpace(q.Default),
		MultiSelect: q.MultiSelect && questionType == "choice",
		Required:    required,
		Preview:     strings.TrimSpace(q.Preview),
		Options:     options,
	}, nil
}

func normalizeOptions(options []askUserOptionReq) []tools.AskUserOption {
	out := make([]tools.AskUserOption, 0, len(options))
	for _, opt := range options {
		value := strings.TrimSpace(opt.Value)
		label := strings.TrimSpace(opt.Label)
		if value == "" && label == "" {
			continue
		}
		if value == "" {
			value = label
		}
		if label == "" {
			label = value
		}
		out = append(out, tools.AskUserOption{
			Value:       value,
			Label:       label,
			Description: strings.TrimSpace(opt.Description),
		})
	}
	return out
}

func normalizeYesNo(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "y", "yes", "true", "1":
		return "yes"
	case "n", "no", "false", "0":
		return "no"
	default:
		return strings.TrimSpace(v)
	}
}

func emitResult(
	events chan<- tools.ToolEvent,
	answersByIndex map[string]string,
	answersByID map[string]string,
	responses []map[string]string,
	canceled bool,
) {
	payload := map[string]any{
		"answers":       answersByIndex,
		"answers_by_id": answersByID,
		"responses":     responses,
		"count":         len(responses),
		"canceled":      canceled,
	}
	if first, ok := answersByIndex["0"]; ok {
		payload["answer"] = first
	}
	body, err := json.Marshal(payload)
	if err != nil {
		events <- tools.NewErrorEvent(err)
		return
	}
	events <- tools.NewOutputEvent(string(body))
}
