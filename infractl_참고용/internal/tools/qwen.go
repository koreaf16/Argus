// Package tools
// File: qwen.go
// Description: Qwen 모델 전용 도구 인덱스 및 가이드라인 생성 헬퍼
// Responsibility: Qwen 인라인 도구 모드에서 사용될 프롬프트 조각 생성

package tools

import (
	"fmt"
	"sort"
	"strings"
)

// AppendQwenToolGuideline 은 Qwen 인라인 도구 모드 사용 가이드라인을 sb 에 추가한다.
func AppendQwenToolGuideline(sb *strings.Builder) {
	sb.WriteString("## Tool Call Format (Qwen Inline Mode)\n")
	sb.WriteString("When using tools, emit XML blocks exactly like this:\n\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString("{\"name\": \"shell_exec\", \"arguments\": {\"command\": \"hostname && uname -r\"}}\n")
	sb.WriteString("</tool_call>\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- The `arguments` object must include all required arguments. Do not call tools with `{}` unless the schema has no required arguments.\n")
	sb.WriteString("- Do not put tool calls inside Markdown code fences.\n")
	sb.WriteString("- After tool responses, produce the final user-facing answer in plain text.\n")
}

// AppendQwenToolIndex 는 도구 목록을 Qwen 인라인 모드용 인덱스 형식으로 sb 에 추가한다.
func AppendQwenToolIndex(sb *strings.Builder, toolList []Tool) {
	sb.WriteString("## Available Tools\n")
	sb.WriteString("Qwen inline-tool mode: use exact tool names and argument keys.\n\n")
	for _, t := range toolList {
		signature := CompactToolSignature(t)
		description := t.Description()
		if mp, ok := t.(ModelPromptDescriber); ok {
			if prompt := strings.TrimSpace(mp.ModelPrompt()); prompt != "" {
				description = prompt
			}
		}
		sb.WriteString(fmt.Sprintf("- `%s`: %s\n", signature, CompactToolDescription(description)))
	}
	sb.WriteString("\n")
}

// CompactToolSignature 는 도구의 이름과 파라미터를 압축된 시그니처 문자열로 반환한다.
func CompactToolSignature(t Tool) string {
	args := compactToolArgs(t.Parameters())
	if args == "" {
		return t.Name() + "()"
	}
	return fmt.Sprintf("%s(%s)", t.Name(), args)
}

func compactToolArgs(params map[string]interface{}) string {
	properties, _ := params["properties"].(map[string]interface{})
	if len(properties) == 0 {
		return ""
	}

	required := map[string]bool{}
	switch raw := params["required"].(type) {
	case []string:
		for _, name := range raw {
			required[name] = true
		}
	case []interface{}:
		for _, item := range raw {
			if name, ok := item.(string); ok {
				required[name] = true
			}
		}
	}

	type argInfo struct {
		name     string
		required bool
		schema   map[string]interface{}
	}

	args := make([]argInfo, 0, len(properties))
	for name, raw := range properties {
		schema, _ := raw.(map[string]interface{})
		args = append(args, argInfo{
			name:     name,
			required: required[name],
			schema:   schema,
		})
	}

	sort.Slice(args, func(i, j int) bool {
		if args[i].required != args[j].required {
			return args[i].required
		}
		return args[i].name < args[j].name
	})

	parts := make([]string, 0, len(args))
	for _, arg := range args {
		part := arg.name
		if !arg.required {
			part += "?"
		}
		part += compactEnumHint(arg.schema)
		parts = append(parts, part)
	}

	return strings.Join(parts, ", ")
}

func compactEnumHint(schema map[string]interface{}) string {
	raw, ok := schema["enum"].([]interface{})
	if !ok || len(raw) == 0 || len(raw) > 3 {
		return ""
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return ""
		}
		values = append(values, value)
	}
	return "=" + strings.Join(values, "|")
}

// CompactToolDescription 은 도구 설명을 한 줄로 압축하여 반환한다.
func CompactToolDescription(description string) string {
	line := strings.TrimSpace(strings.Split(description, "\n")[0])
	line = strings.Join(strings.Fields(line), " ")
	if len(line) <= 96 {
		return line
	}
	return strings.TrimSpace(line[:93]) + "..."
}
