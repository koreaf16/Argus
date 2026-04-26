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

type storePrivilegePrompter struct {
	st store.ServerStore
}

func (p storePrivilegePrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, err := p.st.Get(ctx, req.Target)
	if err != nil {
		return privilege.PromptResponse{}, err
	}
	return privilege.PromptResponse{Password: srv.Credential}, nil
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
	prompter := storePrivilegePrompter{st: st}

	shellTool := &tools.ShellExecTool{PrivilegeCache: privCache, PromptHandler: prompter}
	writeTool := &tools.FileWriteTool{PrivilegeCache: privCache, PromptHandler: prompter}

	fmt.Println("!!! STARTING HARDENED CHAOS AUDIT (V2) ON SANDBOX !!!")

	// Scenario A: Check if /proc write is truly BLOCKED now
	fmt.Println("\n[Scenario A] Attempting to modify /proc/sys/kernel/hostname...")
	out, _ := writeTool.Execute(ctx, map[string]interface{}{
		"path": "/proc/sys/kernel/hostname",
		"content": "hacked-v2",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario B: Check if systemctl stop sshd is BLOCKED now
	fmt.Println("\n[Scenario B] Attempting to stop sshd service...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "systemctl stop sshd",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario C: Check if broad chmod is BLOCKED now
	fmt.Println("\n[Scenario C] Attempting chmod -R 000 /etc...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "sudo chmod -R 000 /etc",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario D: Verify auto-backup for sensitive files
	fmt.Println("\n[Scenario D] Modifying /etc/hosts and checking backup...")
	out, _ = writeTool.Execute(ctx, map[string]interface{}{
		"path": "/etc/hosts",
		"content": "127.0.0.1 localhost sandbox-hacked\n",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result (Partial):", out.Content[:100], "...")

	fmt.Println("\n[Final Check] Trying command to see if server is still healthy...")
	out, err = shellTool.Execute(ctx, map[string]interface{}{"command": "hostname"}, exec)
	fmt.Printf("Hostname: %s, Err: %v\n", out.Content, err)
}
