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
	lineEditTool := &tools.FileLineEditTool{PrivilegeCache: privCache, PromptHandler: prompter}
	diskTool := &tools.DiskUsageTool{}

	fmt.Println("!!! STARTING EXTREME CHAOS AUDIT ON SANDBOX !!!")

	// Scenario A: Blocked Path Check
	fmt.Println("\n[Scenario A] Attempting to modify /proc/sys/kernel/hostname...")
	out, _ := writeTool.Execute(ctx, map[string]interface{}{
		"path": "/proc/sys/kernel/hostname",
		"content": "hacked",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario B: Config Sabotage
	fmt.Println("\n[Scenario B] Sabotaging /etc/shadow...")
	out, _ = lineEditTool.Execute(ctx, map[string]interface{}{
		"path": "/etc/shadow",
		"line": "root:*:12345:0:99999:7:::",
		"regexp": "^root:",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Scenario C: Permission Destruction
	fmt.Println("\n[Scenario C] chmod 000 /etc/ssh/sshd_config...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "chmod 000 /etc/ssh/sshd_config",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result (ExitCode):", out.ExitCode)

	// Scenario D: Resource Exhaustion
	fmt.Println("\n[Scenario D] Filling /tmp/heavy_file (512MB)...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "dd if=/dev/zero of=/tmp/heavy_file bs=1M count=512",
		"become_method": "sudo",
	}, exec)
	fmt.Println("DD ExitCode:", out.ExitCode)
	
	usage, _ := diskTool.Execute(ctx, map[string]interface{}{"path": "/tmp", "mode": "summary"}, exec)
	fmt.Println("Usage after fill:", usage.Content)

	// Scenario E: Network Suicide
	fmt.Println("\n[Scenario E] Stopping sshd service...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "systemctl stop sshd",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Final Attempt
	fmt.Println("\n[Final Check] Trying command after service stop...")
	out, err = shellTool.Execute(ctx, map[string]interface{}{"command": "id"}, exec)
	fmt.Printf("Result: %s, Err: %v\n", out.Content, err)
}
