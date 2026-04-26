// Package tools
// File: file_write_validate.go
// Description: 시스템 설정 파일 수정 전 문법 검증 로직
// Responsibility: sudoers, sysctl.conf 등 치명적인 파일 수정 시 사전 검증 수행

package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ValidateConfig는 특정 파일 경로에 대한 문법 검증 명령어를 반환한다.
func ValidateConfig(exec executor.Executor, path, tempPath string) string {
	base := filepath.Base(path)
	
	switch base {
	case "sudoers":
		// visudo -c -f <temp>
		return fmt.Sprintf("visudo -cf %s", executor.QuotePOSIX(tempPath))
	case "sysctl.conf":
		// sysctl -p <temp> (Note: some sysctl versions might try to apply values)
		return fmt.Sprintf("sysctl -p %s > /dev/null", executor.QuotePOSIX(tempPath))
	case "sshd_config":
		// sshd -t -f <temp>
		return fmt.Sprintf("sshd -t -f %s", executor.QuotePOSIX(tempPath))
	case "fstab":
		// mount -fav (dry run) - but this can be risky depending on version
		return fmt.Sprintf("findmnt --verify --fstab --tab-file %s", executor.QuotePOSIX(tempPath))
	}

	// /etc/sudoers.d/ 하위 파일 처리
	if strings.Contains(path, "/etc/sudoers.d/") {
		return fmt.Sprintf("visudo -cf %s", executor.QuotePOSIX(tempPath))
	}

	return ""
}

// GetValidateErrorHint는 검증 실패 시 사용자에게 보여줄 힌트를 제공한다.
func GetValidateErrorHint(path string) string {
	base := filepath.Base(path)
	return fmt.Sprintf("⚠️ %s 파일의 문법 검증에 실패했습니다. 오타나 잘못된 설정값이 포함되었을 수 있으니 확인 후 다시 시도하십시오.", base)
}
