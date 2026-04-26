// Package tools
// File: args.go
// Description: 도구 인자 맵에서 타입별로 값을 추출하는 헬퍼 함수
// Responsibility: LLM이 생성한 JSON 인자의 안전한 타입 추출

package tools

import (
	"fmt"
	"strconv"
	"strings"
)

// argString은 args 맵에서 key에 해당하는 문자열 값을 반환한다.
// required가 true이고 값이 없으면 error를 반환한다.
func argString(args map[string]interface{}, key string, required bool) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		if required {
			return "", fmt.Errorf("required argument missing: %s", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v), nil
	}
	return s, nil
}

// argInt는 args 맵에서 key에 해당하는 정수 값을 반환한다.
// JSON 숫자는 float64로 파싱되거나, LLM에 의해 문자열로 전달될 수 있으므로 이를 처리한다.
func argInt(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i
		}
	}
	return defaultVal
}

// argStringSlice returns a string slice from args[key].
// It accepts []string, []interface{}, or a single comma/newline-separated string.
func argStringSlice(args map[string]interface{}, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}

	add := func(raw string, out *[]string) {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t'
		}) {
			item := strings.TrimSpace(part)
			if item != "" {
				*out = append(*out, item)
			}
		}
	}

	out := make([]string, 0, 4)
	switch t := v.(type) {
	case []string:
		for _, item := range t {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
	case []interface{}:
		for _, item := range t {
			add(fmt.Sprintf("%v", item), &out)
		}
	case string:
		add(t, &out)
	default:
		add(fmt.Sprintf("%v", t), &out)
	}
	return out
}

// ExtractTarget은 args 맵에서 "target" 값을 추출하고 맵에서 삭제한다.
// 에이전트 루프에서 호출되며, 도구 자체는 target을 처리하지 않는다.
func ExtractTarget(args map[string]interface{}) string {
	v, ok := args["target"]
	if !ok || v == nil {
		return ""
	}
	delete(args, "target")
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// argBool은 args 맵에서 key에 해당하는 불린 값을 반환한다.
func argBool(args map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}
