//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"time"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type resiliencePrompter struct {
	st store.ServerStore
}

func (p *resiliencePrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, _ := p.st.Get(ctx, req.Target)
	return privilege.PromptResponse{Password: srv.Credential}, nil
}

func (p *resiliencePrompter) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	return tools.QuestionResponse{SelectedIndex: 0, SelectedLabel: "Run Anyway"}, nil
}

func main() {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, ".infractl/infractl.db")
	if err != nil { panic(err) }
	defer st.Close()

	srv, _ := st.Get(ctx, "sandbox")
	
	client := sshconn.NewClient(&sshconn.Config{
		Host: srv.Host, Port: srv.Port, User: srv.User, Password: srv.Credential, 
		AuthType: string(srv.AuthType), WorkspaceDir: srv.WorkspaceDir,
		Timeout: 5 * time.Second,
	})
	defer client.Close()
	
	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	privCache := privilege.NewCache()
	apprCache := tools.NewApprovalCache()
	prompter := &resiliencePrompter{st: st}

	shellTool := &tools.ShellExecTool{
		PrivilegeCache: privCache, ApprovalCache: apprCache, PromptHandler: prompter,
		IsYoloMode: func() bool { return true },
	}

	fmt.Println("🔥 !!! STARTING EXTREME RESILIENCE AUDIT !!! 🔥")

	// --- [Step 1: Binary Sabotage] ---
	fmt.Println("\n[Step 1] Sabotaging 'ls' binary...")
	shellTool.Execute(ctx, map[string]interface{}{
		"command": "sudo mv /usr/bin/ls /usr/bin/ls.broken",
	}, exec)
	
	res, _ := exec.Execute(ctx, "ls")
	if res.ExitCode != 0 {
		fmt.Println("💀 Confirmed: 'ls' is dead.")
	}

	// --- [Step 2: The Reboot Challenge] ---
	fmt.Println("\n[Step 2] Issuing REBOOT command...")
	go func() {
		shellTool.Execute(ctx, map[string]interface{}{
			"command": "sudo reboot",
		}, exec)
	}()

	fmt.Println("⏳ Server is going down. Waiting for reboot and reconnection...")
	time.Sleep(10 * time.Second)
	client.Close()

	// --- [Step 3: Auto-Reconnect & Recovery] ---
	var finalExec *sshconn.SSHExecutor
	var success bool
	
	for i := 0; i < 30; i++ { // Retry for 150 seconds
		fmt.Printf("Attempt %d: Reaching %s...\n", i+1, srv.Host)
		newClient := sshconn.NewClient(&sshconn.Config{
			Host: srv.Host, Port: srv.Port, User: srv.User, Password: srv.Credential, 
			AuthType: string(srv.AuthType), WorkspaceDir: srv.WorkspaceDir,
			Timeout: 3 * time.Second,
		})
		
		// Run "id" to check connection
		runRes, err := newClient.Run(ctx, "id")
		if err == nil && runRes.ExitCode == 0 {
			fmt.Println("✨ RECONNECTED SUCCESSFULLY!")
			finalExec = sshconn.NewSSHExecutor(srv.Name, newClient, srv.OS, srv.WorkspaceDir)
			success = true
			break
		}
		time.Sleep(5 * time.Second)
	}

	if !success {
		fmt.Println("❌ FAILED: Could not reconnect after reboot.")
		return
	}

	// Now recovery!
	fmt.Println("\n[Step 4] Recovering 'ls' binary...")
	shellTool.PrivilegeCache = privilege.NewCache()
	
	recoverRes, _ := shellTool.Execute(ctx, map[string]interface{}{
		"command": "sudo mv /usr/bin/ls.broken /usr/bin/ls",
	}, finalExec)
	
	fmt.Println("Recovery Result:", recoverRes.Content)
	
	verifyRes, _ := finalExec.Execute(ctx, "ls -l /usr/bin/ls")
	if verifyRes.ExitCode == 0 {
		fmt.Println("🏆 ULTIMATE SUCCESS: Server rebooted and 'ls' binary restored!")
	} else {
		fmt.Println("💥 PARTIAL FAILURE: Reconnected, but could not fix 'ls'.")
	}

	fmt.Println("\n🔥 RESILIENCE AUDIT COMPLETE. 🔥")
}
