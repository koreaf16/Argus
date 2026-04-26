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
		Timeout:      15 * time.Second,
		Password:     srv.Credential,
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	
	privCache := privilege.NewCache()
	prompter := storePrivilegePrompter{st: st}

	shellTool := &tools.ShellExecTool{PrivilegeCache: privCache, PromptHandler: prompter}
	writeTool := &tools.FileWriteTool{PrivilegeCache: privCache, PromptHandler: prompter}
	lineEditTool := &tools.FileLineEditTool{PrivilegeCache: privCache, PromptHandler: prompter}
	diskTool := &tools.DiskUsageTool{}

	fmt.Println("!!! STARTING EXTREME CHAOS AUDIT ON SANDBOX !!!")
	fmt.Println("Target:", srv.Host, "User:", srv.User)

	// Scenario A: Attempt to write to BLOCKED virtual FS
	fmt.Println("\n[Scenario A] Attempting to modify /proc/sys/kernel/hostname via file_write...")
	out, _ := writeTool.Execute(ctx, map[string]interface{}{
		"path": "/proc/sys/kernel/hostname",
		"content": "hacked-sandbox",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario B: Sabotage /etc/shadow (Warned but allowed?)
	fmt.Println("\n[Scenario B] Sabotaging /etc/shadow via file_line_edit...")
	out, _ = lineEditTool.Execute(ctx, map[string]interface{}{
		"path": "/etc/shadow",
		"line": "root:*:12345:0:99999:7:::",
		"regexp": "^root:",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario C: Connectivity Suicide (Destroying SSH permissions)
	fmt.Println("\n[Scenario C] Making /etc/ssh/sshd_config unreadable...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "chmod 000 /etc/ssh/sshd_config && ls -l /etc/ssh/sshd_config",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario D: Disk Filling (Resource Exhaustion)
	// We fill /tmp with 1GB file (adjust size if needed)
	fmt.Println("\n[Scenario D] Filling up disk in /tmp...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "dd if=/dev/zero of=/tmp/heavy_file bs=1M count=1024 status=progress",
		"become_method": "sudo",
	}, exec)
	fmt.Println("DD Result:", out.Content)
	
	usage, _ := diskTool.Execute(ctx, map[string]interface{}{"path": "/tmp", "mode": "summary"}, exec)
	fmt.Println("Usage after fill:", usage.Content)

	// Scenario E: Critical Service Kill
	fmt.Println("\n[Scenario E] Stopping sshd service (Final Suicide)...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "systemctl stop sshd",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Verification: Can we still run a command?
	fmt.Println("\n[Verification] Attempting follow-up command after sshd stop...")
	out, err = shellTool.Execute(ctx, map[string]interface{}{"command": "echo 'Still alive?'"}, exec)
	fmt.Printf("Result: %s, Err: %v\n", out.Content, err)

	fmt.Println("\n!!! CHAOS AUDIT FINISHED !!!")
	fmt.Println("Check if you can still access the server.")
}
