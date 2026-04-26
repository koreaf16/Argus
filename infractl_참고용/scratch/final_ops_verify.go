//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"strings"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
	"github.com/yourorg/infractl/internal/checkpoint"
)

// 시뮬레이션용 프롬프터: "Approve for 5m"을 자동으로 선택
type finalPrompter struct {
	st           store.ServerStore
	lastQuestion string
}

func (p *finalPrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, _ := p.st.Get(ctx, req.Target)
	return privilege.PromptResponse{Password: srv.Credential}, nil
}

func (p *finalPrompter) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	p.lastQuestion = req.Question
	fmt.Printf("[UI PROMPT] Question: %s\n", req.Header)
	// Simulate choosing "Approve for 5m" (SelectedIndex: 1)
	fmt.Println("[UI CHOICE] Selecting: Approve for 5m")
	return tools.QuestionResponse{SelectedIndex: 1, SelectedLabel: "Approve for 5m"}, nil
}

func main() {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, ".infractl/infractl.db")
	if err != nil { panic(err) }
	defer st.Close()

	srv, _ := st.Get(ctx, "sandbox")
	client := sshconn.NewClient(&sshconn.Config{
		Host: srv.Host, Port: srv.Port, User: srv.User, Password: srv.Credential, WorkspaceDir: srv.WorkspaceDir,
	})
	defer client.Close()
	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)

	privCache := privilege.NewCache()
	apprCache := tools.NewApprovalCache()
	prompter := &finalPrompter{st: st}
	cpMgr := checkpoint.NewManager(st)

	sysctlTool := &tools.SysctlManageTool{PrivilegeCache: privCache, PromptHandler: prompter, ApprovalCache: apprCache}
	writeTool := &tools.FileWriteTool{PrivilegeCache: privCache, PromptHandler: prompter, ApprovalCache: apprCache, CheckpointManager: cpMgr}

	fmt.Println("=== [Test 1] sysctl_manage & Session Approval ===")
	// Initial call: Should prompt
	out, _ := sysctlTool.Execute(ctx, map[string]interface{}{
		"key": "net.ipv4.ip_forward", "value": "1", "become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	fmt.Println("\n=== [Test 2] Session Trust (No Prompt Expected) ===")
	prompter.lastQuestion = ""
	// Second call on same target: Should NOT prompt
	out, _ = writeTool.Execute(ctx, map[string]interface{}{
		"path": "/etc/sysctl.conf", "content": "# InfraCtl Session Trust Test\n", "append": true, "become_method": "sudo",
	}, exec)
	if prompter.lastQuestion == "" {
		fmt.Println("✓ Success: Session trust maintained, no redundant prompts.")
	} else {
		fmt.Println("✗ Fail: Prompt was unexpectedly shown again.")
	}

	fmt.Println("\n=== [Test 3] Syntax Validation (Invalid Config) ===")
	// Some OS's sysctl is lenient, but totally invalid lines usually fail
	fmt.Println("Attempting to write invalid config to /etc/sysctl.conf...")
	out, _ = writeTool.Execute(ctx, map[string]interface{}{
		"path": "/etc/sysctl.conf", "content": "this_is_clearly_invalid_sysctl_syntax!!!", "append": false, "become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)
	if strings.Contains(out.Content, "[Validation Error]") {
		fmt.Println("✓ Success: Syntax validation correctly blocked the destructive write.")
	}

	fmt.Println("\n=== [Test 4] Automatic Checkpoint Verification ===")
	latest, _ := cpMgr.GetLatest(ctx, srv.Name)
	if latest.ID > 0 {
		fmt.Printf("✓ Success: Automatic checkpoint found: [%d] %s\n", latest.ID, latest.Description)
	} else {
		fmt.Println("✗ Fail: No checkpoint found.")
	}

	// Final Cleanup
	exec.Execute(ctx, "sudo sed -i '/InfraCtl Session Trust Test/d' /etc/sysctl.conf")
}
