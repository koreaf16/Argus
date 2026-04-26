// Package precheck
// File: checker_service.go
// Description: systemctl 명령 실행 전 서비스 unit 파일 존재 여부와 현재 상태를 검증하는 Checker 구현
// Responsibility: systemctl list-unit-files와 is-active를 사용해 서비스 존재 및 상태를 확인한다.

package precheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ServiceExistsChecker verifies that a systemd service unit file exists on the remote host
// and, for start/stop/restart actions, checks whether the service is currently active.
type ServiceExistsChecker struct {
	service  string
	action   string
	severity Severity
}

// NewServiceExistsChecker constructs a ServiceExistsChecker for the given service and action.
func NewServiceExistsChecker(service, action string, sev Severity) *ServiceExistsChecker {
	return &ServiceExistsChecker{service: service, action: action, severity: sev}
}

func (c *ServiceExistsChecker) Kind() CheckKind    { return KindServiceExists }
func (c *ServiceExistsChecker) Subject() string    { return c.service }
func (c *ServiceExistsChecker) Severity() Severity { return c.severity }

// Run checks that the service unit file exists, then validates the current active state
// against the requested action.
func (c *ServiceExistsChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	svc := strings.TrimSpace(c.service)
	if svc == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.service, OK: true, Severity: c.severity}
	}

	// Step 1: check unit file existence.
	unitCmd := fmt.Sprintf(
		"systemctl list-unit-files --type=service 2>/dev/null | grep -q %s",
		shellQuote("^"+svc+"\\.service"),
	)
	unitResult, err := exec.Execute(ctx, unitCmd)
	if err != nil {
		// Cannot determine — let it through.
		return CheckResult{Kind: c.Kind(), Subject: c.service, OK: true, Severity: c.severity}
	}
	if unitResult.ExitCode != 0 {
		return CheckResult{
			Kind:     c.Kind(),
			Subject:  c.service,
			OK:       false,
			Severity: c.severity,
			Message:  fmt.Sprintf("service unit %q not found on this system", svc),
		}
	}

	// Step 2: validate active state vs. action.
	action := strings.ToLower(strings.TrimSpace(c.action))
	activeCmd := fmt.Sprintf("systemctl is-active %s 2>/dev/null", shellQuote(svc))
	activeResult, err := exec.Execute(ctx, activeCmd)
	if err != nil {
		return CheckResult{Kind: c.Kind(), Subject: c.service, OK: true, Severity: c.severity}
	}

	isActive := activeResult.ExitCode == 0

	switch action {
	case "stop", "restart":
		if !isActive {
			return CheckResult{
				Kind:     c.Kind(),
				Subject:  c.service,
				OK:       false,
				Severity: c.severity,
				Message:  fmt.Sprintf("service %q is not currently active", svc),
			}
		}
	case "start":
		if isActive {
			return CheckResult{
				Kind:     c.Kind(),
				Subject:  c.service,
				OK:       false,
				Severity: c.severity,
				Message:  fmt.Sprintf("service %q is already running", svc),
			}
		}
	}

	return CheckResult{Kind: c.Kind(), Subject: c.service, OK: true, Severity: c.severity}
}
