// Package powershell — Dangerous PowerShell cmdlet detection.
//
// 파일 역할: Identifies PowerShell cmdlets that are destructive or dangerous.
//
//	Maintains a set of cmdlets requiring extra permission/validation.
//
// 포함 모듈:
//   - DangerousPowerShellCmdlets: Set of dangerous cmdlet names
//   - IsDangerousCmdlet: Checks if a cmdlet is in the dangerous set
//
// 호출/사용 방식:
//   - tools/powershell uses this to validate commands before execution
//   - internal/utils/permissions for permission checks
//
// 연결:
//   - internal/utils/powershell: parser.go
//   - 원본: src/utils/powershell/ danger detection
package powershell

import (
	"strings"
)

// DangerousPowerShellCmdlets is the set of PowerShell cmdlets that are destructive.
var DangerousPowerShellCmdlets = map[string]bool{
	// File/directory deletion
	"remove-item":         true,
	"remove-itemsecurity": true,
	"del":                 true,
	"rmdir":               true,
	"erase":               true,

	// Process management
	"stop-process":   true,
	"kill":           true,
	"remove-process": true,
	"taskkill":       true,

	// User/account management
	"remove-localuser":  true,
	"remove-aduser":     true,
	"disable-localuser": true,
	"unlock-adaccount":  true,
	"set-aduser":        true,

	// Registry modification
	"new-itemproperty":    true,
	"set-itemproperty":    true,
	"remove-itemproperty": true,

	// System shutdown
	"restart-computer": true,
	"stop-computer":    true,
	"shutdown":         true,

	// Service management
	"stop-service":   true,
	"remove-service": true,
	"set-service":    true,

	// Network configuration
	"remove-netroute":    true,
	"remove-netneighbor": true,
	"set-netadapter":     true,

	// SQL/database operations (destructive)
	"invoke-sqlcmd": true,
	"drop-database": true,

	// Formatting/clearing
	"clear-content": true,
	"set-content":   true,
	"add-content":   true,

	// Permissions/security
	"set-acl":           true,
	"remove-acl":        true,
	"grant-permission":  true,
	"revoke-permission": true,

	// Group policy
	"invoke-gpupdate":  true,
	"set-gppermission": true,

	// BitLocker/encryption
	"disable-bitlocker": true,
	"resume-bitlocker":  true,
	"suspend-bitlocker": true,

	// Firewall
	"remove-netfirewallrule":  true,
	"set-netfirewallrule":     true,
	"new-netfirewallrule":     true,
	"disable-netfirewallrule": true,

	// Event log
	"clear-eventlog":  true,
	"remove-eventlog": true,

	// Windows Update
	"remove-windowsupdate":  true,
	"disable-windowsupdate": true,

	// Other destructive operations
	"format":    true,
	"diskpart":  true,
	"cipher":    true,
	"cipher /w": true,
	"cipher /x": true,
}

// IsDangerousCmdlet checks if a PowerShell cmdlet is in the dangerous set.
// Performs case-insensitive matching.
func IsDangerousCmdlet(cmdlet string) bool {
	if cmdlet == "" {
		return false
	}

	// Normalize to lowercase for comparison
	normalized := strings.ToLower(cmdlet)

	// Check direct match
	if DangerousPowerShellCmdlets[normalized] {
		return true
	}

	// Check for alias variations (e.g., "del" for "Remove-Item")
	// Aliases are handled by normalizing both cmdlet and alias names
	return false
}

// DangerousPatterns are regex-like patterns for dangerous operations.
var DangerousPatterns = []string{
	"(Remove|Delete|Clear).*",
	"Stop-.*",
	"Disable-.*",
	"Restart-Computer",
	"Stop-Computer",
	"Set-Acl",
	"Remove-Acl",
	"Invoke-.*Command",
	"Invoke-WebRequest.*-OutFile.*",
	"[System.IO.File]::Delete",
	"[System.IO.Directory]::Delete",
	"rm ",    // Unix-style rm
	"rmdir ", // rmdir
}

// IsPatternDangerous checks if a command matches a dangerous pattern.
// Phase 1: Simple string matching for common patterns.
func IsPatternDangerous(command string) bool {
	dangerousKeywords := []string{
		"Remove-Item",
		"Remove-ItemProperty",
		"Delete",
		"Clear-Content",
		"Stop-Process",
		"Stop-Service",
		"Stop-Computer",
		"Restart-Computer",
		"Set-Acl",
		"Remove-Acl",
		"Format",
		"Cipher",
		"DiskPart",
	}

	lowerCmd := strings.ToLower(command)
	for _, keyword := range dangerousKeywords {
		if strings.Contains(lowerCmd, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}
