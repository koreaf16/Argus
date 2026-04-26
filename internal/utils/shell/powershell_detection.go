// Package shell — PowerShell 바이너리 탐색 및 에디션 감지.
//
// 파일 역할: PATH 에서 pwsh/powershell 을 찾아 경로를 반환한다.
//
//	Linux 에서 snap 런처를 피하는 로직을 포함하며, sync.Once 로 결과를 캐싱한다.
//
// 포함 모듈:
//   - PowerShellEdition: "core" | "desktop" 에디션 타입
//   - FindPowerShell: PATH 에서 PowerShell 바이너리 탐색
//   - GetCachedPowerShellPath: 메모이즈된 탐색 결과 반환
//   - GetPowerShellEdition: 바이너리 이름으로 에디션 추론
//   - ResetPowerShellCache: 테스트용 캐시 초기화
//
// 호출/사용 방식:
//   - CreatePowerShellProvider 에서 shellPath 결정 시 사용
//   - internal/tools/powershell 에서 에디션별 프롬프트 생성 시 사용
//
// 연결:
//   - 원본: src/utils/shell/powershellDetection.ts
package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// PowerShellEdition 은 PowerShell 에디션을 나타낸다.
type PowerShellEdition string

const (
	// PSEditionCore 는 PowerShell 7+(pwsh) 에디션이다.
	PSEditionCore PowerShellEdition = "core"
	// PSEditionDesktop 은 Windows PowerShell 5.1(powershell) 에디션이다.
	PSEditionDesktop PowerShellEdition = "desktop"
)

var (
	psOnce       sync.Once
	psCachedPath string
	psCachedErr  error
)

// FindPowerShell 은 PATH 에서 PowerShell 바이너리를 탐색해 경로를 반환한다.
// pwsh (Core 7+) 를 먼저 시도하고, 없으면 powershell (Desktop 5.1) 을 시도한다.
// Linux 에서 snap 런처(/snap/) 경로는 /opt/microsoft/powershell/7/pwsh 로 대체한다.
func FindPowerShell() (string, error) {
	pwshPath, err := exec.LookPath("pwsh")
	if err == nil {
		if runtime.GOOS == "linux" {
			resolved, rerr := filepath.EvalSymlinks(pwshPath)
			if rerr != nil {
				resolved = pwshPath
			}
			if strings.HasPrefix(pwshPath, "/snap/") || strings.HasPrefix(resolved, "/snap/") {
				// snap 런처를 피하고 직접 바이너리를 사용한다.
				for _, candidate := range []string{
					"/opt/microsoft/powershell/7/pwsh",
					"/usr/bin/pwsh",
				} {
					if isExecutableFile(candidate) {
						directResolved, _ := filepath.EvalSymlinks(candidate)
						if !strings.HasPrefix(candidate, "/snap/") && !strings.HasPrefix(directResolved, "/snap/") {
							return candidate, nil
						}
					}
				}
			}
		}
		return pwshPath, nil
	}

	powershellPath, err := exec.LookPath("powershell")
	if err == nil {
		return powershellPath, nil
	}

	return "", err
}

// isExecutableFile 은 경로가 존재하는 일반 파일인지 확인한다.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// GetCachedPowerShellPath 는 메모이즈된 PowerShell 경로를 반환한다.
// 첫 호출 시 FindPowerShell 을 실행하고 이후 결과를 재사용한다.
func GetCachedPowerShellPath() (string, error) {
	psOnce.Do(func() {
		psCachedPath, psCachedErr = FindPowerShell()
	})
	return psCachedPath, psCachedErr
}

// GetPowerShellEdition 은 캐시된 바이너리 이름에서 에디션을 추론한다.
// pwsh/pwsh.exe → core, powershell/powershell.exe → desktop.
func GetPowerShellEdition() (PowerShellEdition, error) {
	p, err := GetCachedPowerShellPath()
	if err != nil || p == "" {
		return "", err
	}
	base := filepath.Base(p)
	base = strings.ToLower(strings.TrimSuffix(base, ".exe"))
	if base == "pwsh" {
		return PSEditionCore, nil
	}
	return PSEditionDesktop, nil
}

// ResetPowerShellCache 는 캐시를 초기화한다. 테스트에서만 사용한다.
func ResetPowerShellCache() {
	psOnce = sync.Once{}
	psCachedPath = ""
	psCachedErr = nil
}
