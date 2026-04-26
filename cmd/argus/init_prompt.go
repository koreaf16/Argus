// Package main — init menu prompt and selection helpers.
//
// 파일 역할: `argus --init` 대화형 메뉴에서 사용하는 입력 처리,
//
//	provider 선택, alias 정규화, 다중 선택 파싱을 분리한다.
//
// 포함 모듈:
//   - prompt/promptRequired/promptInt/confirm: 기본 입력 프롬프트.
//   - promptProvider: provider 선택.
//   - uniqueModelAlias/parseIndexSelection/sanitizeAlias: import helper.
//
// 호출/사용 방식:
//   - cmd/argus/init_menu.go의 initSession 메서드들이 호출한다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): services/llm.
//   - 이 파일을 import 하는 주요 패키지: 없음.
package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/koreaf16/argus/internal/services/llm"
)

func (s *initSession) promptProvider() (llm.Provider, error) {
	fmt.Fprintln(s.out, titleStyle.Render("Select Provider:"))
	fmt.Fprintln(s.out, menuItemStyle.Render("1. openai-compat"))
	fmt.Fprintln(s.out, menuItemStyle.Render("2. anthropic"))
	fmt.Fprintln(s.out, menuItemStyle.Render("3. gemini"))
	raw, err := s.prompt("provider", "1")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "openai", "openai-compat", "openai_compat":
		return llm.ProviderOpenAICompat, nil
	case "2", "anthropic":
		return llm.ProviderAnthropic, nil
	case "3", "gemini":
		return llm.ProviderGemini, nil
	default:
		return "", fmt.Errorf("invalid provider: %s", raw)
	}
}

func (s *initSession) prompt(label, def string) (string, error) {
	labelStyled := promptStyle.Render(label)
	if def == "" {
		fmt.Fprintf(s.out, " %s: ", labelStyled)
	} else {
		fmt.Fprintf(s.out, " %s [%s]: ", labelStyled, dimStyle.Render(def))
	}
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

func (s *initSession) promptRequired(label, def string) (string, error) {
	for {
		value, err := s.prompt(label, def)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
		fmt.Fprintln(s.out, " ! value is required.")
	}
}

func (s *initSession) promptInt(label string, def int, required bool) (int, error) {
	defText := ""
	if def > 0 {
		defText = strconv.Itoa(def)
	}
	for {
		value, err := s.prompt(label, defText)
		if err != nil {
			return 0, err
		}
		if strings.TrimSpace(value) == "" && !required {
			return 0, nil
		}
		number, convErr := strconv.Atoi(strings.TrimSpace(value))
		if convErr != nil || number <= 0 {
			fmt.Fprintln(s.out, " ! enter a positive integer.")
			continue
		}
		return number, nil
	}
}

func (s *initSession) confirm(label string, def bool) (bool, error) {
	labelStyled := promptStyle.Render(label)
	suffix := "[y/N]"
	if def {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(s.out, " %s %s: ", labelStyled, dimStyle.Render(suffix))
	value, err := s.reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return def, nil
	}
	switch strings.ToLower(value) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return def, nil
	}
}

func (s *initSession) uniqueModelAlias(base string) string {
	base = sanitizeAlias(base)
	if base == "" {
		base = "model"
	}
	if _, exists := s.registry.Get(base); !exists {
		return base
	}
	for i := 2; ; i++ {
		alias := fmt.Sprintf("%s-%d", base, i)
		if _, exists := s.registry.Get(alias); !exists {
			return alias
		}
	}
}

func parseIndexSelection(raw string, size int) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(raw, "all") {
		indexes := make([]int, 0, size)
		for i := 0; i < size; i++ {
			indexes = append(indexes, i)
		}
		return indexes, nil
	}

	seen := map[int]struct{}{}
	out := make([]int, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid index: %s", part)
		}
		if n < 1 || n > size {
			return nil, fmt.Errorf("index out of range: %d", n)
		}
		if _, ok := seen[n-1]; ok {
			continue
		}
		seen[n-1] = struct{}{}
		out = append(out, n-1)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no indexes selected")
	}
	return out, nil
}

func sanitizeAlias(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}
