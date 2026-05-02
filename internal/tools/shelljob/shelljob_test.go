package shelljob

import (
	"testing"

	"github.com/koreaf16/argus/internal/shelljobs"
	tool "github.com/koreaf16/argus/internal/tools"
)

func TestShellJobToolVisibleOnlyWhenJobsExist(t *testing.T) {
	tl := NewShellJobTool()
	ctx := tool.Context{ShellJobs: shelljobs.NewManager()}

	if tl.IsVisible(ctx) {
		t.Fatalf("shell_job should be hidden when there are no background jobs")
	}

	ctx.ShellJobs.StartJob("bash", "local", "sleep 10", nil, nil, "")
	if !tl.IsVisible(ctx) {
		t.Fatalf("shell_job should be visible when a background job exists")
	}
}
