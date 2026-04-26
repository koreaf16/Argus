// Package bash — bash 명령 프리픽스 추출.
//
// 파일 역할: 명령 문자열에서 권한 규칙 매칭에 사용할 프리픽스를 추출한다.
//
//	내부적으로 파서와 레지스트리, shell 패키지의 BuildPrefix 를 사용한다.
//
// 포함 모듈:
//   - GetBashCommandPrefix: 명령 문자열에서 프리픽스를 비동기 추출
//   - GetCompoundCommandPrefixes: 복합 명령(&&/||/;)의 프리픽스 목록 반환
//
// 호출/사용 방식:
//   - internal/utils/permissions/bash_classifier.go 가 권한 판단 전에 호출
//
// 연결:
//   - 원본: src/utils/bash/prefix.ts
//   - import: internal/utils/shell (BuildPrefix)
//   - import: internal/utils/bash (ParseCommand, GetCommandSpec)
package bash

import (
	"context"
	"strings"

	"github.com/koreaf16/argus/internal/utils/shell"
)

// GetBashCommandPrefix 는 command 에서 권한 규칙 매칭용 프리픽스를 반환한다.
// 파싱에 실패하거나 단순한 명령이면 nil 을 반환한다.
func GetBashCommandPrefix(ctx context.Context, command string) (string, error) {
	parsed := ParseCommand(command)
	if parsed == nil {
		return "", nil
	}
	if parsed.CommandNode == nil {
		return "", nil
	}

	args := ExtractCommandArguments(parsed.CommandNode)
	if len(args) == 0 {
		return "", nil
	}

	cmd := args[0]
	restArgs := args[1:]

	prefix := shell.BuildPrefix(cmd, restArgs)
	if prefix == "" {
		return "", nil
	}

	// 환경변수 접두사 포함
	if len(parsed.EnvVars) > 0 {
		envPrefix := strings.Join(parsed.EnvVars, " ")
		return envPrefix + " " + prefix, nil
	}
	return prefix, nil
}

// GetCompoundCommandPrefixes 는 복합 명령(&&, ||, ;)을 분리해 각 서브커맨드의 프리픽스를 반환한다.
func GetCompoundCommandPrefixes(ctx context.Context, command string) ([]string, error) {
	subcommands := SplitCommandDeprecated(command)
	if len(subcommands) <= 1 {
		prefix, err := GetBashCommandPrefix(ctx, command)
		if err != nil || prefix == "" {
			return nil, err
		}
		return []string{prefix}, nil
	}

	var prefixes []string
	for _, sub := range subcommands {
		trimmed := strings.TrimSpace(sub)
		if trimmed == "" {
			continue
		}
		prefix, err := GetBashCommandPrefix(ctx, trimmed)
		if err != nil {
			return nil, err
		}
		if prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes, nil
}
