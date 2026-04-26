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

type inspectPrompter struct {
	st store.ServerStore
}

func (p inspectPrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, err := p.st.Get(ctx, req.Target)
	if err != nil {
		return privilege.PromptResponse{}, err
	}
	return privilege.PromptResponse{Password: srv.Credential}, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

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
		Timeout:      30 * time.Second,
	}
	if srv.AuthType == store.AuthTypeKey {
		cfg.KeyPath = srv.Credential
	} else {
		cfg.Password = srv.Credential
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()
	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	tool := &tools.ShellExecTool{PrivilegeCache: privilege.NewCache(), PromptHandler: inspectPrompter{st: st}}

	command := `
echo "=== File Count in Target Directory ==="
find /home/oracle/app/oracle/product/19c/dbhome_1 -type f | wc -l

echo -e "\n=== Directory Structure (First 20) ==="
ls -la /home/oracle/app/oracle/product/19c/dbhome_1 | head -20

echo -e "\n=== Check Ownership of Root-owned Files ==="
find /home/oracle/app/oracle/product/19c/dbhome_1 -user root | head -10
`

	out, err := tool.Execute(ctx, map[string]interface{}{
		"command":       command,
		"become_method": "sudo",
		"become_user":   "root",
		"description":   "diagnose unzip failure",
	}, exec)
	
	if err != nil {
		fmt.Printf("Execution failed: %v\n", err)
		return
	}
	
	fmt.Printf("Success: %v, ExitCode: %d\n", out.Success, out.ExitCode)
	fmt.Println(out.Content)
	if out.ErrorMessage != "" {
		fmt.Printf("Error: %s\n", out.ErrorMessage)
	}
}
