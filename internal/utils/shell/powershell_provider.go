package shell

import (
	"context"
	"encoding/base64"
	"fmt"
	"unicode/utf16"
)

type powerShellProvider struct {
	shellPath string
}

func CreatePowerShellProvider(shellPath string) ShellProvider {
	return &powerShellProvider{shellPath: shellPath}
}

func (p *powerShellProvider) ProviderType() ShellType { return ShellTypePS }
func (p *powerShellProvider) ShellPath() string       { return p.shellPath }
func (p *powerShellProvider) IsDetached() bool        { return false }

func (p *powerShellProvider) BuildExecCommand(_ context.Context, command string, opts ExecCommandOpts) (ExecCommandResult, error) {
	psCommand := command

	var commandString string
	if opts.UseSandbox {
		escapedShellPath := escapePOSIXSingleQuote(p.shellPath)
		commandString = fmt.Sprintf("'%s' -NoProfile -NonInteractive -EncodedCommand %s", escapedShellPath, encodePSCommand(psCommand))
	} else {
		commandString = psCommand
	}

	return ExecCommandResult{CommandString: commandString}, nil
}

func (p *powerShellProvider) GetSpawnArgs(commandString string) []string {
	return buildPowerShellArgs(commandString)
}

func (p *powerShellProvider) GetEnvironmentOverrides(_ context.Context, _ string) (map[string]string, error) {
	return map[string]string{}, nil
}

func buildPowerShellArgs(cmd string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-Command", cmd}
}

func encodePSCommand(cmd string) string {
	runes := []rune(cmd)
	u16 := utf16.Encode(runes)
	buf := make([]byte, len(u16)*2)
	for i, r := range u16 {
		buf[i*2] = byte(r)
		buf[i*2+1] = byte(r >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func escapePOSIXSingleQuote(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			result = append(result, '\'', '\\', '\'', '\'')
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}
