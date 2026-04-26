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
		Timeout:      10 * time.Second,
		Password:     srv.Credential,
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	
	shellTool := &tools.ShellExecTool{
		PrivilegeCache: privilege.NewCache(),
		PromptHandler:  storePrivilegePrompter{st: st},
	}
	diskTool := &tools.DiskUsageTool{}

	fmt.Printf("--- OS Operation Verification on %s ---\n", srv.Name)

	// 1. Capacity Check
	fmt.Println("\n[1] Checking Disk Usage...")
	out, _ := diskTool.Execute(ctx, map[string]interface{}{"path": "/", "mode": "summary"}, exec)
	fmt.Println(out.Content)

	// 2. Create User (Sudo)
	fmt.Println("\n[2] Creating testuser...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       "id testuser || useradd -m testuser",
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println(out.Content)

	// 3. Su to testuser (using sudo -u)
	fmt.Println("\n[3] Testing 'sudo -u testuser'...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       "whoami && pwd",
		"become_method": "sudo",
		"become_user":   "testuser",
	}, exec)
	fmt.Println(out.Content)

	// 4. File Write & Permission Check
	fmt.Println("\n[4] Writing file as testuser...")
	// We need to make sure the directory is writable by testuser
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       "mkdir -p /tmp/test_dir && chown testuser:testuser /tmp/test_dir",
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	
	// Let's use shell_exec for writing as specific user for now
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       "echo 'hello world' > /tmp/test_dir/hello.txt && chmod 644 /tmp/test_dir/hello.txt",
		"become_method": "sudo",
		"become_user":   "testuser",
	}, exec)
	fmt.Println("File created via sudo -u.")

	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "ls -l /tmp/test_dir/hello.txt",
	}, exec)
	fmt.Println(out.Content)

	// 4.1. Extraction (Tar) Check
	fmt.Println("\n[4.1] Testing tar extraction...")
	tarCmd := "cd /tmp/test_dir && tar -cvf hello.tar hello.txt && mkdir -p extracted && tar -xvf hello.tar -C extracted"
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       tarCmd,
		"become_method": "sudo",
		"become_user":   "testuser",
	}, exec)
	fmt.Println(out.Content)
	
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "ls -R /tmp/test_dir/extracted",
	}, exec)
	fmt.Println("Extracted content:")
	fmt.Println(out.Content)

	// 5. Idempotent sysctl.conf modification
	fmt.Println("\n[5] Idempotent sysctl.conf modification...")
	sysctlLine := "fs.file-max = 65535"
	// Pattern: grep -qF "line" file || echo "line" >> file
	// We use become_method: sudo to write to /etc/sysctl.conf
	cmd := fmt.Sprintf("grep -qF '%s' /etc/sysctl.conf || echo '%s' >> /etc/sysctl.conf", sysctlLine, sysctlLine)
	
	fmt.Println("First attempt...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       cmd,
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println(out.Content)

	fmt.Println("Second attempt (should be no-op)...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       cmd,
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println(out.Content)
	
	out, _ = shellTool.Execute(ctx, map[string]interface{}{"command": "grep 'fs.file-max' /etc/sysctl.conf"}, exec)
	fmt.Println("Result in sysctl.conf:", out.Content)

	// 6. Cleanup
	fmt.Println("\n[6] Cleanup...")
	shellTool.Execute(ctx, map[string]interface{}{
		"command":       "rm -rf /tmp/test_dir && userdel -r testuser",
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	// Remove sysctl line for cleanliness
	shellTool.Execute(ctx, map[string]interface{}{
		"command":       fmt.Sprintf("sed -i '/%s/d' /etc/sysctl.conf", strings.ReplaceAll(sysctlLine, ".", "\\.")),
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println("Done.")
}
