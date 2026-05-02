package tools

import (
	"fmt"
	"strings"
)

// ValidateTextOnlyCapable rejects known GUI installers unless a silent option is present.
func ValidateTextOnlyCapable(shellType, command string) error {
	_ = shellType
	cmd := strings.TrimSpace(command)
	lowerCmd := strings.ToLower(cmd)

	guiTools := map[string][]string{
		"runinstaller": {"-silent"},
		"setup.exe":    {"-silent", "/s", "/quiet"},
		"install.exe":  {"-silent", "/s", "/quiet"},
	}

	for toolName, requiredOptions := range guiTools {
		if !strings.Contains(lowerCmd, toolName) {
			continue
		}
		for _, opt := range requiredOptions {
			if strings.Contains(lowerCmd, opt) {
				return nil
			}
		}
		return fmt.Errorf("command %q requires a GUI. In CLI environments, include one of these silent-mode options: %s", toolName, strings.Join(requiredOptions, " or "))
	}

	return nil
}

// ValidateCommandIntegrity catches incomplete shell syntax before execution.
func ValidateCommandIntegrity(shellType, command string) error {
	_ = shellType
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return fmt.Errorf("command is empty")
	}

	if strings.HasSuffix(cmd, ">") || strings.HasSuffix(cmd, ">>") || strings.HasSuffix(cmd, "<") {
		return fmt.Errorf("command ends with a redirection operator; target path is missing")
	}

	if strings.HasSuffix(cmd, "|") {
		return fmt.Errorf("command ends with a pipe operator; next command is missing")
	}

	quotes := []rune{'"', '\''}
	for _, q := range quotes {
		count := strings.Count(cmd, string(q))
		escapedCount := strings.Count(cmd, "\\"+string(q))
		if (count-escapedCount)%2 != 0 {
			return fmt.Errorf("unbalanced quote: %c", q)
		}
	}

	return nil
}
