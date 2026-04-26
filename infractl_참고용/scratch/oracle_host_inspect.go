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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	command := `set +e
echo "== users =="
getent passwd | awk -F: 'tolower($1) ~ /(oracle|grid|ora|db)/ {print $1 ":" $3 ":" $6 ":" $7}'
echo "== processes =="
ps -ef | egrep '[p]mon|[t]nslsnr|[o]ra_|[o]racle' | head -80
echo "== listeners =="
ss -ltnp 2>/dev/null | egrep ':1521|:1522|:2484' || true
echo "== sqlplus paths =="
for base in /u01 /opt /home /app /oracle; do
  [ -d "$base" ] && find "$base" -maxdepth 7 -type f -name sqlplus -perm /111 2>/dev/null
done | sort -u | head -40
echo "== oracle dirs =="
for p in /u01 /opt/oracle /home/oracle /app/oracle /oracle; do
  [ -e "$p" ] && ls -ld "$p"
done
echo "== systemd units =="
systemctl list-units --type=service --all --no-pager 2>/dev/null | egrep -i 'oracle|listener|tns|database|db' | head -80 || true`

	out, err := tool.Execute(ctx, map[string]interface{}{
		"command":       command,
		"become_method": "sudo",
		"become_user":   "root",
		"description":   "oracle host inventory",
	}, exec)
	fmt.Printf("success=%v exit=%d err=%v\n", out.Success, out.ExitCode, err)
	fmt.Println(strings.TrimSpace(out.Content))
	if strings.TrimSpace(out.ErrorMessage) != "" {
		fmt.Println("[error]", strings.TrimSpace(out.ErrorMessage))
	}
}
