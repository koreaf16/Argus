package shelljobcontrol

import (
	"testing"

	"github.com/koreaf16/argus/internal/shelljobs"
	tool "github.com/koreaf16/argus/internal/tools"
)

func TestShellJobControlToolVisibleOnlyWhenJobsExist(t *testing.T) {
	tl := NewShellJobControlTool()
	ctx := tool.Context{ShellJobs: shelljobs.NewManager()}

	if tl.IsVisible(ctx) {
		t.Fatalf("shell_job_control should be hidden when there are no background jobs")
	}

	ctx.ShellJobs.StartJob("bash", "local", "sleep 10", nil, nil, "")
	if !tl.IsVisible(ctx) {
		t.Fatalf("shell_job_control should be visible when a background job exists")
	}
}
