// Package builtins
// File: assets.go
// Description: builtin hook 자원(.sh, .md) 을 //go:embed 로 내장하고 ~/.infractl/builtins/ 로 언박싱한다.
// Responsibility: 내장 자산 노출 API + 세션 시작 시 파일 시스템 복사.

// claude_cli 대응: 없음 (claude_cli 는 hook 자산을 배포하지 않고 사용자가 직접 작성).
// InfraCtl 은 기본 보안 정책 보장을 위해 minimal bundle (.sh 1개 + .md 1개) 을 bundle.

package builtins

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed system_risk.sh system_risk.ps1 agent/shell_validator.md
var assetsFS embed.FS

// ShellValidatorPrompt 는 shell_validator.md 의 전체 텍스트를 반환한다.
// prompt backend 의 $BUILTIN:shell_validator 해결에 사용된다.
func ShellValidatorPrompt() (string, error) {
	b, err := assetsFS.ReadFile("agent/shell_validator.md")
	if err != nil {
		return "", fmt.Errorf("read embedded shell_validator.md: %w", err)
	}
	return string(b), nil
}

// LookupPrompt 는 이름으로 내장 프롬프트를 찾는다. $BUILTIN:<name> 해결용.
// name 예: "shell_validator".
func LookupPrompt(name string) (string, bool) {
	switch name {
	case "shell_validator":
		s, err := ShellValidatorPrompt()
		if err != nil {
			return "", false
		}
		return s, true
	}
	return "", false
}

// Unbox 는 모든 내장 자산을 dstDir 에 복사한다. 이미 존재하는 파일은 덮어쓴다 (업그레이드 허용).
// dstDir 예: ~/.infractl/builtins. 디렉토리는 없으면 생성한다.
func Unbox(dstDir string) error {
	if dstDir == "" {
		return fmt.Errorf("unbox: empty destination")
	}

	return fs.WalkDir(assetsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk embedded fs %q: %w", path, err)
		}
		if path == "." {
			return nil
		}

		dst := filepath.Join(dstDir, path)
		if d.IsDir() {
			if mkErr := os.MkdirAll(dst, 0o755); mkErr != nil {
				return fmt.Errorf("mkdir %s: %w", dst, mkErr)
			}
			return nil
		}

		data, rdErr := assetsFS.ReadFile(path)
		if rdErr != nil {
			return fmt.Errorf("read embedded %s: %w", path, rdErr)
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return fmt.Errorf("mkdir parent of %s: %w", dst, mkErr)
		}
		// .sh 는 실행권한 부여
		mode := os.FileMode(0o644)
		if filepath.Ext(path) == ".sh" {
			mode = 0o755
		}
		if wErr := os.WriteFile(dst, data, mode); wErr != nil {
			return fmt.Errorf("write %s: %w", dst, wErr)
		}
		return nil
	})
}

// ScriptPath 는 언박싱된 스크립트의 절대 경로를 돌려준다.
// hooks.yaml 에서 `~/.infractl/builtins/system_risk.sh` 로 참조되므로,
// 기본적으로는 Unbox 결과 경로를 직접 사용하면 된다. 이 헬퍼는 테스트/내부 호출용.
func ScriptPath(dstDir, name string) string {
	return filepath.Join(dstDir, name)
}
