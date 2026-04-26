// Package infrainit
// File: hooks.go
// Description: 세션 시작 시 hooks.yaml + builtins 자산을 ~/.infractl/ 로 부트스트랩한다.
// Responsibility: 최초 실행 시 기본 정책 파일 배포, 업그레이드 시 builtins 만 새로고침.

// 설계 원칙:
//   - hooks.yaml: 사용자가 편집할 수 있으므로 이미 존재하면 절대 덮어쓰지 않는다.
//   - builtins/*: InfraCtl 이 관리하는 자산이므로 매 실행마다 최신 버전으로 교체한다 (보안 패치 반영).

package infrainit

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/yourorg/infractl/internal/hooks/builtins"
	"github.com/yourorg/infractl/internal/hooks/defaults"
)

// BootstrapHooks 는 cfgDir(~/.infractl) 아래 hooks.yaml 과 builtins/ 를 준비한다.
//   - hooks.yaml 이 없으면 embed 기본값을 복사한다 (이미 있으면 건드리지 않음).
//   - builtins/ 은 항상 최신 embed 자산으로 덮어쓴다.
//
// 에러는 warn 로그로 남기고 반환한다. 부트스트랩 실패가 InfraCtl 기동을 막지 않도록
// 호출 측에서 nil 반환을 무시하는 것도 허용된다.
func BootstrapHooks(cfgDir string) error {
	if cfgDir == "" {
		return fmt.Errorf("bootstrap hooks: empty cfgDir")
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfgDir, err)
	}

	// 1) hooks.yaml — 최초 1회만 설치
	yamlPath := filepath.Join(cfgDir, "hooks.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		data := defaults.DefaultHooksYAML()
		if wErr := os.WriteFile(yamlPath, data, 0o644); wErr != nil {
			return fmt.Errorf("write default hooks.yaml: %w", wErr)
		}
		slog.Info("bootstrapped default hooks.yaml", "path", yamlPath)
	} else if err != nil {
		return fmt.Errorf("stat hooks.yaml: %w", err)
	}

	// 2) builtins/ — 매번 새로고침 (InfraCtl 관리 자산)
	builtinsDir := filepath.Join(cfgDir, "builtins")
	if err := builtins.Unbox(builtinsDir); err != nil {
		return fmt.Errorf("unbox builtins: %w", err)
	}
	slog.Debug("builtins refreshed", "dir", builtinsDir)

	return nil
}
