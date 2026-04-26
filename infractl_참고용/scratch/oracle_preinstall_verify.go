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

	fmt.Printf("--- Oracle Pre-install Verification on %s ---\n", srv.Name)

	// 1. Group Creation
	fmt.Println("\n[1] Creating groups (oinstall, dba)...")
	out, _ := shellTool.Execute(ctx, map[string]interface{}{
		"command":       "groupadd oinstall || true; groupadd dba || true",
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println(out.Content)

	// 2. User Creation with multiple groups
	fmt.Println("\n[2] Creating oracle user with groups...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       "id oracle || useradd -m -g oinstall -G dba oracle",
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println(out.Content)

	// 3. Idempotent block edit for /etc/security/limits.conf
	fmt.Println("\n[3] Idempotent limits.conf modification...")
	limitsBlock := `
# Oracle settings start
oracle soft nofile 1024
oracle hard nofile 65536
oracle soft nproc 2047
oracle hard nproc 16384
oracle soft stack 10240
# Oracle settings end`
	
	// We use a marker to identify the block
	marker := "# Oracle settings start"
	
	// Use printf with %q to handle newlines correctly in shell
	// Actually, easier to use a temp file and cat it.
	script := fmt.Sprintf(`
if ! grep -qF '%s' /etc/security/limits.conf; then
  cat <<'EOF' >> /etc/security/limits.conf
%s
EOF
  echo "Limits added."
else
  echo "Limits already exist."
fi
`, marker, strings.TrimSpace(limitsBlock))

	fmt.Println("Attempting block edit...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command":       script,
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println(out.Content)

	fmt.Println("Verifying limits.conf...")
	out, _ = shellTool.Execute(ctx, map[string]interface{}{
		"command": "tail -n 10 /etc/security/limits.conf",
	}, exec)
	fmt.Println(out.Content)

	// 4. Cleanup
	fmt.Println("\n[4] Cleanup...")
	shellTool.Execute(ctx, map[string]interface{}{
		"command":       "userdel -r oracle; groupdel oinstall; groupdel dba",
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	shellTool.Execute(ctx, map[string]interface{}{
		"command":       fmt.Sprintf("sed -i '/%s/,/%s/d' /etc/security/limits.conf", "# Oracle settings start", "# Oracle settings end"),
		"become_method": "sudo",
		"become_user":   "root",
	}, exec)
	fmt.Println("Done.")
}
