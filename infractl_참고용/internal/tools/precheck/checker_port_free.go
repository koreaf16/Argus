// Package precheck
// File: checker_port_free.go
// Description: 지정된 TCP 포트의 LISTEN 여부를 원격 호스트에서 사전 검증하는 Checker 구현
// Responsibility: ss 또는 netstat을 사용해 포트가 이미 사용 중인지 확인한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// PortFreeChecker verifies that a TCP port is not already bound on the remote host.
// Tries ss first; falls back to netstat if ss is unavailable.
type PortFreeChecker struct {
	port     int
	severity Severity
}

// NewPortFreeChecker constructs a PortFreeChecker for the given TCP port number.
func NewPortFreeChecker(port int, sev Severity) *PortFreeChecker {
	return &PortFreeChecker{port: port, severity: sev}
}

func (c *PortFreeChecker) Kind() CheckKind    { return KindPortFree }
func (c *PortFreeChecker) Subject() string    { return fmt.Sprintf("%d", c.port) }
func (c *PortFreeChecker) Severity() Severity { return c.severity }

// Run checks whether the port is already in LISTEN state using ss or netstat.
func (c *PortFreeChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	// Try ss first; fall back to netstat.
	cmd := fmt.Sprintf(
		`ss -tlnp 2>/dev/null | grep -q ":%d " || { netstat -tlnp 2>/dev/null | grep -q ":%d "; }; [ $? -eq 0 ] && echo "IN_USE" || echo "FREE"`,
		c.port, c.port,
	)
	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		// Cannot determine — skip.
		return CheckResult{Kind: c.Kind(), Subject: c.Subject(), OK: true, Severity: c.severity}
	}

	if result.ExitCode != 0 {
		return CheckResult{Kind: c.Kind(), Subject: c.Subject(), OK: true, Severity: c.severity}
	}

	output := result.Stdout
	// If "IN_USE" appears in output the port is occupied.
	inUse := false
	for _, line := range splitLines(output) {
		if line == "IN_USE" {
			inUse = true
			break
		}
	}

	if !inUse {
		return CheckResult{Kind: c.Kind(), Subject: c.Subject(), OK: true, Severity: c.severity}
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.Subject(),
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"port %d is already in use — stop the existing process before starting the listener",
			c.port,
		),
	}
}

// splitLines splits a string into non-empty trimmed lines.
func splitLines(s string) []string {
	var out []string
	for _, line := range splitNewlines(s) {
		if t := trimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitNewlines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func trimSpace(s string) string {
	result := s
	for len(result) > 0 && (result[0] == ' ' || result[0] == '\t' || result[0] == '\r') {
		result = result[1:]
	}
	for len(result) > 0 {
		last := result[len(result)-1]
		if last == ' ' || last == '\t' || last == '\r' {
			result = result[:len(result)-1]
		} else {
			break
		}
	}
	return result
}
