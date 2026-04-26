// Package precheck
// File: checker_host_reachable.go
// Description: curl/wget 실행 전 대상 호스트의 TCP 도달 가능성을 사전 검증하는 Checker 구현
// Responsibility: nc 또는 /dev/tcp bash fallback을 사용해 host:port 연결 가능 여부를 확인한다.

package precheck

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// HostReachableChecker verifies that a remote host is reachable on a given TCP port
// before a curl/wget command attempts to connect to it.
type HostReachableChecker struct {
	host     string
	port     int
	severity Severity
}

// NewHostReachableChecker constructs a HostReachableChecker for the given host and port.
// port is accepted as a string (from URL parsing) and converted internally.
func NewHostReachableChecker(host, portStr string, sev Severity) *HostReachableChecker {
	p, _ := strconv.Atoi(portStr)
	return &HostReachableChecker{host: host, port: p, severity: sev}
}

func (c *HostReachableChecker) Kind() CheckKind    { return KindHostReachable }
func (c *HostReachableChecker) Subject() string    { return fmt.Sprintf("%s:%d", c.host, c.port) }
func (c *HostReachableChecker) Severity() Severity { return c.severity }

// Run tries nc first; falls back to bash /dev/tcp if nc is absent.
func (c *HostReachableChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	host := strings.TrimSpace(c.host)
	if host == "" || c.port <= 0 {
		return CheckResult{Kind: c.Kind(), Subject: c.Subject(), OK: true, Severity: c.severity}
	}

	portStr := strconv.Itoa(c.port)

	// Try nc; fall back to bash /dev/tcp.
	cmd := fmt.Sprintf(
		"nc -zw5 %s %s 2>/dev/null || bash -c 'echo >/dev/tcp/%s/%s' 2>/dev/null",
		shellQuote(host), portStr,
		shellQuote(host), portStr,
	)
	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		// Cannot determine — let it through.
		return CheckResult{Kind: c.Kind(), Subject: c.Subject(), OK: true, Severity: c.severity}
	}

	if result.ExitCode == 0 {
		return CheckResult{Kind: c.Kind(), Subject: c.Subject(), OK: true, Severity: c.severity}
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.Subject(),
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"host %q port %d is not reachable — check network connectivity or firewall rules",
			host, c.port,
		),
	}
}

// parseURLHostPort extracts (host, port, ok) from a URL string without using net/url.
// Supports http, https, and ftp schemes with optional explicit ports.
func parseURLHostPort(rawURL string) (host, port string, ok bool) {
	// Strip scheme.
	var rest string
	switch {
	case strings.HasPrefix(rawURL, "https://"):
		rest = rawURL[len("https://"):]
		port = "443"
	case strings.HasPrefix(rawURL, "http://"):
		rest = rawURL[len("http://"):]
		port = "80"
	case strings.HasPrefix(rawURL, "ftp://"):
		rest = rawURL[len("ftp://"):]
		port = "21"
	default:
		return "", "", false
	}

	// Strip path: everything after the first '/'.
	authority := rest
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		authority = rest[:idx]
	}

	// Strip userinfo (user:pass@host).
	if idx := strings.LastIndexByte(authority, '@'); idx >= 0 {
		authority = authority[idx+1:]
	}

	// Handle IPv6 addresses: [::1]:8080.
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return "", "", false
		}
		h := authority[1:end]
		tail := authority[end+1:]
		if strings.HasPrefix(tail, ":") {
			port = tail[1:]
		}
		if h == "" {
			return "", "", false
		}
		return h, port, true
	}

	// Regular host[:port].
	if h, p, found := strings.Cut(authority, ":"); found {
		if h == "" {
			return "", "", false
		}
		return h, p, true
	}

	if authority == "" {
		return "", "", false
	}
	return authority, port, true
}
