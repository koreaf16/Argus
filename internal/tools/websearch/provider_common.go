package websearch

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := row[key]
		if !ok || v == nil {
			continue
		}
		switch typed := v.(type) {
		case string:
			if s := strings.TrimSpace(typed); s != "" {
				return s
			}
		default:
			s := strings.TrimSpace(fmt.Sprintf("%v", typed))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func numberString(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return ""
		}
		return strconv.FormatInt(int64(n), 10)
	case float32:
		if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
			return ""
		}
		return strconv.FormatInt(int64(n), 10)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	case string:
		if s := strings.TrimSpace(n); s != "" {
			return s
		}
	}
	return ""
}

func appendSnippetText(parts []string, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return parts
	}
	return append(parts, text)
}

func appendSnippetKV(parts []string, key, value string) []string {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return parts
	}
	return append(parts, key+"="+value)
}

func appendSnippetFlag(parts []string, flag string, enabled bool) []string {
	if !enabled {
		return parts
	}
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return parts
	}
	return append(parts, flag)
}

func joinSnippet(parts []string) string {
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		trimmed = append(trimmed, part)
	}
	return strings.Join(trimmed, ", ")
}
