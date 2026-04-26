//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// 시뮬레이션용 프롬프터: 특정 조건에 따라 자동 승인 또는 취소
type chaosPrompter struct {
	st           store.ServerStore
	forcedChoice int // 0: Run Anyway, 1: Cancel
}

func (p chaosPrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, _ := p.st.Get(ctx, req.Target)
	return privilege.PromptResponse{Password: srv.Credential}, nil
}

func (p chaosPrompter) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	fmt.Printf("[SIMULATED UI] Question: %s\n", req.Question)
	labels := []string{"Run Anyway", "Cancel"}
	fmt.Printf("[SIMULATED UI] Choosing: %s\n", labels[p.forcedChoice])
	return tools.QuestionResponse{SelectedIndex: p.forcedChoice, SelectedLabel: labels[p.forcedChoice]}, nil
}

func main() {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, ".infractl/infractl.db")
	if err != nil {
		panic(err)
	}
	defer st.Close()

	srv, err := st.Get(ctx, "sandbox")
	if err != nil {
		panic(err)
	}

	cfg := &sshconn.Config{
		Host:         srv.Host,
		Port:         srv.Port,
		User:         srv.User,
		AuthType:     string(srv.AuthType),
		WorkspaceDir: srv.WorkspaceDir,
		Timeout:      20 * time.Second,
		Password:     srv.Credential,
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	
	privCache := privilege.NewCache()

	fmt.Println("!!! STARTING INTERACTIVE CHAOS AUDIT (V3) ON SANDBOX !!!")

	// --- [Scenario A: Run Anyway] ---
	fmt.Println("\n[Scenario A] Attempting to modify /proc with 'Run Anyway' choice...")
	
	writeTool := &tools.FileWriteTool{
		PrivilegeCache: privCache,
		PromptHandler:  chaosPrompter{st: st, forcedChoice: 0}, // Index 0: Run Anyway
	}
	
	out, _ := writeTool.Execute(ctx, map[string]interface{}{
		"path": "/proc/sys/kernel/hostname",
		"content": "hacked-via-confirm",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Verify hostname change
	hostOut, _ := exec.Execute(ctx, "cat /proc/sys/kernel/hostname")
	fmt.Printf("Current Hostname on Server: %s\n", strings.TrimSpace(hostOut.Stdout))

	// --- [Scenario B: Cancel] ---
	fmt.Println("\n[Scenario B] Attempting to stop sshd with 'Cancel' choice...")

	shellTool := &tools.ShellExecTool{
		PrivilegeCache: privCache,
		PromptHandler:  chaosPrompter{st: st, forcedChoice: 1}, // Index 1: Cancel
	}

	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "systemctl stop sshd",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Final verification
	fmt.Println("\n[Final Check] Server is still reachable and healthy.")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{"command": "id"}, exec)
	fmt.Println("ID Result:", strings.TrimSpace(out.Content))
	
	// Cleanup hostname
	exec.Execute(ctx, "echo 'localhost.localdomain' | sudo tee /proc/sys/kernel/hostname")
}
