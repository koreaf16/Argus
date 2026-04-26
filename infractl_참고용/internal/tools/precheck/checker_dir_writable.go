package precheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// DirWritableChecker verifies that the nearest existing ancestor is writable.
type DirWritableChecker struct {
	dir      string
	severity Severity
}

// NewDirWritableChecker constructs a DirWritableChecker for the given remote directory path.
func NewDirWritableChecker(dir string, sev Severity) *DirWritableChecker {
	return &DirWritableChecker{dir: dir, severity: sev}
}

func (c *DirWritableChecker) Kind() CheckKind    { return KindDirWritable }
func (c *DirWritableChecker) Subject() string    { return c.dir }
func (c *DirWritableChecker) Severity() Severity { return c.severity }

// Run finds the nearest existing ancestor and verifies that it is writable.
func (c *DirWritableChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	dir := strings.TrimSpace(c.dir)
	if dir == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.dir, OK: true, Severity: c.severity}
	}

	ancestorCmd := fmt.Sprintf(`ancestor=%s
while [ ! -d "$ancestor" ] && [ "$ancestor" != "/" ]; do
  ancestor=$(dirname "$ancestor")
done
printf '%%s\n' "$ancestor"`, shellQuote(dir))
	ancestorResult, err := exec.Execute(ctx, ancestorCmd)
	ancestor := dir
	if err == nil {
		if checked := strings.TrimSpace(ancestorResult.Stdout); checked != "" {
			ancestor = checked
		}
	}

	writeCmd := fmt.Sprintf("test -w %s", shellQuote(ancestor))
	writeResult, writeErr := exec.Execute(ctx, writeCmd)
	if writeErr == nil && writeResult.ExitCode == 0 {
		return CheckResult{Kind: c.Kind(), Subject: c.dir, OK: true, Severity: c.severity}
	}

	ownerCmd := fmt.Sprintf("stat -c '%%U' %s 2>/dev/null", shellQuote(ancestor))
	ownerResult, _ := exec.Execute(ctx, ownerCmd)
	owner := strings.TrimSpace(ownerResult.Stdout)

	msg := fmt.Sprintf(
		"directory %q is not writable by the current user (checked ancestor %q) - re-run as the directory owner (sudo -u %s ...)",
		dir, ancestor, owner,
	)
	if owner == "" {
		msg = fmt.Sprintf(
			"directory %q is not writable by the current user (checked ancestor %q) - check ownership and permissions",
			dir, ancestor,
		)
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.dir,
		OK:       false,
		Severity: c.severity,
		Message:  msg,
	}
}
