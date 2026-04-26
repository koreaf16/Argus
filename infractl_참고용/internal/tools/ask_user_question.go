// Package tools
// File: ask_user_question.go
// Description: Ask a question to the user and receive structured feedback.
// Responsibility: Allow the LLM to clarify ambiguous targets or requests via the TUI.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

type AskUserQuestionTool struct {
	// IsYoroMode, when non-nil, reports whether YORO mode is currently active.
	// In YORO mode the tool auto-proceeds without asking the user.
	IsYoroMode func() bool
}

func (t *AskUserQuestionTool) Name() string {
	return "ask_user_question"
}

func (t *AskUserQuestionTool) Description() string {
	return "사용자에게 지금 꼭 필요한 단일 질문을 던집니다. 선택형이면 실제 답변 후보만 options에 넣고, 자유응답이면 options를 생략하세요."
}

func (t *AskUserQuestionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{
				"type":        "string",
				"description": "사용자에게 보여줄 단일 질문 내용",
			},
			"options": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"label":       map[string]interface{}{"type": "string", "description": "선택지 라벨"},
						"description": map[string]interface{}{"type": "string", "description": "선택지 설명"},
					},
					"required": []string{"label", "description"},
				},
				"minItems":    2,
				"maxItems":    5,
				"description": "서로 배타적인 실제 답변 후보 리스트",
			},
			"allow_custom_input": map[string]interface{}{
				"type":        "boolean",
				"description": "선택지 외 자유 입력 허용 여부",
			},
			"header": map[string]interface{}{
				"type":        "string",
				"description": "질문 UI 상단 헤더",
			},
			"fields": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":          map[string]interface{}{"type": "string", "description": "필드 식별자"},
						"label":         map[string]interface{}{"type": "string", "description": "필드 라벨"},
						"placeholder":   map[string]interface{}{"type": "string", "description": "텍스트 입력 placeholder"},
						"required":      map[string]interface{}{"type": "boolean", "description": "필수 여부"},
						"default_value": map[string]interface{}{"type": "string", "description": "기본값"},
						"field_type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"text", "select"},
							"description": "입력 방식",
						},
						"options": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "field_type=select일 때의 선택지 목록",
						},
					},
					"required": []string{"name", "label"},
				},
				"minItems":    1,
				"description": "여러 입력값을 한 번에 받을 때 사용하는 폼 정의",
			},
		},
		"required": []string{"question"},
	}
}

func (t *AskUserQuestionTool) IsReadOnly() bool {
	return true
}

func (t *AskUserQuestionTool) IsEnabled() bool {
	return true
}

func (t *AskUserQuestionTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	if rawFields, exists := args["fields"]; exists && rawFields != nil {
		return t.executeForm(ctx, args, rawFields)
	}

	question, ok := args["question"].(string)
	if !ok || strings.TrimSpace(question) == "" {
		question = "Clarification needed. Please answer the following question:"
	}

	var optsRaw []interface{}
	if raw, exists := args["options"]; exists && raw != nil {
		var ok bool
		optsRaw, ok = raw.([]interface{})
		if !ok {
			return ToolOutcome{Content: "Error: missing or invalid options", Success: true}, nil
		}
		if len(optsRaw) < 2 || len(optsRaw) > 5 {
			return ToolOutcome{Content: "Error: options must contain 2-5 items when provided", Success: true}, nil
		}
	}

	options := make([]QuestionOption, len(optsRaw))
	for i, o := range optsRaw {
		switch v := o.(type) {
		case map[string]interface{}:
			label, _ := v["label"].(string)
			desc, _ := v["description"].(string)
			label = strings.TrimSpace(label)
			desc = strings.TrimSpace(desc)
			if label == "" {
				return ToolOutcome{Content: fmt.Sprintf("Error: missing label at index %d", i), Success: true}, nil
			}
			options[i] = QuestionOption{Label: label, Description: desc}
		case string:
			label := strings.TrimSpace(v)
			if label == "" {
				return ToolOutcome{Content: fmt.Sprintf("Error: empty string option at index %d", i), Success: true}, nil
			}
			options[i] = QuestionOption{Label: label}
		default:
			return ToolOutcome{Content: fmt.Sprintf(`Error: option at index %d must be an object {"label":"...","description":"..."} or a plain string`, i), Success: true}, nil
		}
	}

	allowCustomInput, _ := args["allow_custom_input"].(bool)
	if len(options) == 0 {
		allowCustomInput = true
	}

	header, _ := args["header"].(string)
	header = strings.TrimSpace(header)
	if header == "" {
		header = "Clarification Needed"
	}

	req := QuestionRequest{
		Question:         strings.TrimSpace(question),
		Options:          options,
		AllowCustomInput: allowCustomInput,
		Header:           header,
	}

	if t.IsYoroMode != nil && t.IsYoroMode() {
		return ToolOutcome{
			Content: "YOLO mode is active. Please evaluate the context yourself and automatically proceed with the appropriate next steps without asking the user. Make the best technical judgment and execute the corresponding tool calls directly.",
			Success: true,
		}, nil
	}

	resp, err := RequestQuestion(ctx, req)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: user interaction failed: %s", err), Success: true}, nil
	}

	if resp.IsOther {
		if strings.TrimSpace(resp.UserInput) == "" {
			return ToolOutcome{Content: "User chose custom input but did not provide a value.", Success: true}, nil
		}
		return ToolOutcome{Content: fmt.Sprintf("User provided input: %s", resp.UserInput), Success: true}, nil
	}

	if resp.SelectedIndex < 0 {
		return ToolOutcome{Content: "User canceled the question without choice.", Success: true}, nil
	}

	return ToolOutcome{Content: fmt.Sprintf("User selected option %d: %s (%s)", resp.SelectedIndex+1, resp.SelectedLabel, resp.UserInput), Success: true}, nil
}

// executeForm handles multi-field structured input requests.
func (t *AskUserQuestionTool) executeForm(ctx context.Context, args map[string]interface{}, rawFields interface{}) (ToolOutcome, error) {
	fieldsArr, ok := rawFields.([]interface{})
	if !ok || len(fieldsArr) == 0 {
		return ToolOutcome{Content: "Error: fields must be a non-empty array", Success: true}, nil
	}

	fields := make([]FormFieldDef, 0, len(fieldsArr))
	for i, rawField := range fieldsArr {
		m, ok := rawField.(map[string]interface{})
		if !ok {
			return ToolOutcome{Content: fmt.Sprintf("Error: invalid field at index %d", i), Success: true}, nil
		}
		name, _ := m["name"].(string)
		label, _ := m["label"].(string)
		name = strings.TrimSpace(name)
		label = strings.TrimSpace(label)
		if name == "" || label == "" {
			return ToolOutcome{Content: fmt.Sprintf("Error: field at index %d missing name or label", i), Success: true}, nil
		}
		placeholder, _ := m["placeholder"].(string)
		required, _ := m["required"].(bool)
		defaultValue, _ := m["default_value"].(string)
		fieldType, _ := m["field_type"].(string)
		var options []string
		if rawOpts, ok := m["options"].([]interface{}); ok {
			for _, o := range rawOpts {
				if s, ok := o.(string); ok && s != "" {
					options = append(options, s)
				}
			}
		}
		fields = append(fields, FormFieldDef{
			Name:         name,
			Label:        label,
			Placeholder:  placeholder,
			Required:     required,
			DefaultValue: defaultValue,
			FieldType:    fieldType,
			Options:      options,
		})
	}

	title, _ := args["question"].(string)
	title = strings.TrimSpace(title)
	header, _ := args["header"].(string)
	header = strings.TrimSpace(header)

	req := FormRequest{
		Title:  title,
		Fields: fields,
		Header: header,
	}

	if t.IsYoroMode != nil && t.IsYoroMode() {
		return ToolOutcome{
			Content: "YOLO mode is active. Please evaluate the context yourself and automatically proceed with the appropriate next steps without asking the user. Make the best technical judgment and execute the corresponding tool calls directly.",
			Success: true,
		}, nil
	}

	resp, err := RequestForm(ctx, req)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: form interaction failed: %s", err), Success: true}, nil
	}

	if resp.Cancelled {
		return ToolOutcome{Content: "User cancelled the form without submitting.", Success: true}, nil
	}

	var parts []string
	for _, field := range fields {
		val := resp.Values[field.Name]
		parts = append(parts, fmt.Sprintf("%s=%s", field.Name, val))
	}
	return ToolOutcome{Content: fmt.Sprintf("Form submitted: %s", strings.Join(parts, ", ")), Success: true}, nil
}
