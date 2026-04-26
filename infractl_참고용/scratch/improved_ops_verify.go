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
		Timeout:      10 * time.Second,
		Password:     srv.Credential,
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	
	privCache := privilege.NewCache()
	prompter := storePrivilegePrompter{st: st}

	lineEditTool := &tools.FileLineEditTool{
		PrivilegeCache: privCache,
		PromptHandler:  prompter,
	}

	fmt.Printf("--- FileLineEditTool Verification on %s ---\n", srv.Name)

	// Test 1: Add new line
	fmt.Println("\n[1] Adding new line to /tmp/test_limits.conf...")
	exec.Execute(ctx, "echo '# Initial file' > /tmp/test_limits.conf")
	
	out, _ := lineEditTool.Execute(ctx, map[string]interface{}{
		"path": "/tmp/test_limits.conf",
		"line": "oracle soft nofile 1024",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Test 2: Idempotency (Add same line again)
	fmt.Println("\n[2] Adding same line again (idempotency check)...")
	out, _ = lineEditTool.Execute(ctx, map[string]interface{}{
		"path": "/tmp/test_limits.conf",
		"line": "oracle soft nofile 1024",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Test 3: Replace using regexp
	fmt.Println("\n[3] Replacing line using regexp...")
	out, _ = lineEditTool.Execute(ctx, map[string]interface{}{
		"path":   "/tmp/test_limits.conf",
		"regexp": "^oracle soft nofile",
		"line":   "oracle soft nofile 2048",
	}, exec)
	fmt.Println("Result:", out.Content)

	// Test 4: Verify with file_read (should also support become)
	fmt.Println("\n[4] Verifying with FileReadTool (become: sudo)...")
	readTool := &tools.FileReadTool{PrivilegeCache: privCache, PromptHandler: prompter}
	out, _ = readTool.Execute(ctx, map[string]interface{}{
		"path": "/tmp/test_limits.conf",
	}, exec)
	fmt.Println("File content:\n", out.Content)

	// Cleanup
	exec.Execute(ctx, "rm -f /tmp/test_limits.conf")
	fmt.Println("\nDone.")
}
