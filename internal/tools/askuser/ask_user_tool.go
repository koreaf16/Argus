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
	return "Ask the user one or more interactive questions and return the answers."
}

func (t *AskUserTool) InputSchema() tools.ToolInputJSONSchema {
	questionSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":           map[string]any{"type": "string"},
			"header":       map[string]any{"type": "string"},
			"question":     map[string]any{"type": "string"},
			"type":         map[string]any{"type": "string", "enum": []string{"text", "choice", "yesno"}},
			"placeholder":  map[string]any{"type": "string"},
			"default":      map[string]any{"type": "string"},
			"multi_select": map[string]any{"type": "boolean"},
			"required":     map[string]any{"type": "boolean"},
			"options": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value":       map[string]any{"type": "string"},
						"label":       map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
					},
				},
			},
		},
		"required": []string{"question"},
	}

	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"question":     map[string]any{"type": "string"},
			"header":       map[string]any{"type": "string"},
			"type":         map[string]any{"type": "string", "enum": []string{"text", "choice", "yesno"}},
			"placeholder":  map[string]any{"type": "string"},
			"default":      map[string]any{"type": "string"},
			"multi_select": map[string]any{"type": "boolean"},
			"required":     map[string]any{"type": "boolean"},
			"options":      questionSchema["properties"].(map[string]any)["options"],
			"questions": map[string]any{
				"type":  "array",
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
			if q.Required && value == "" {
				events <- tools.NewErrorEvent(fmt.Errorf("question %q requires an answer", q.Question))
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

		emitResult(events, answersByIndex, answersByID, responses, false)
		events <- tools.NewDoneEvent()
	}()
	return events, nil
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
		return nil, fmt.Errorf("invalid ask_user input: %w", err)
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
			Options:     req.Options,
		})
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("ask_user requires at least one question")
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
		return tools.AskUserQuestion{}, fmt.Errorf("questions[%d].question is required", index)
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
			return tools.AskUserQuestion{}, fmt.Errorf("questions[%d].options is required for type=choice", index)
		}
	case "yesno":
		if len(options) == 0 {
			options = []tools.AskUserOption{
				{Value: "yes", Label: "Yes"},
				{Value: "no", Label: "No"},
			}
		}
	default:
		return tools.AskUserQuestion{}, fmt.Errorf("questions[%d].type must be one of: text, choice, yesno", index)
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
