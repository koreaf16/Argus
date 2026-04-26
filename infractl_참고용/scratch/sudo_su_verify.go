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
	if srv.AuthType != store.AuthTypePassword || strings.TrimSpace(srv.Credential) == "" {
		return privilege.PromptResponse{}, fmt.Errorf("no stored password for %s", req.Target)
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
	}
	if srv.AuthType == store.AuthTypeKey {
		cfg.KeyPath = srv.Credential
	} else {
		cfg.Password = srv.Credential
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	tool := &tools.ShellExecTool{
		PrivilegeCache: privilege.NewCache(),
		PromptHandler:  storePrivilegePrompter{st: st},
	}

	fmt.Printf("target=%s host=%s user=%s os=%s workspace=%s\n", srv.Name, srv.Host, srv.User, srv.OS, srv.WorkspaceDir)

	runDirect := func(name, cmd string, timeout time.Duration) {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		start := time.Now()
		res, err := exec.Execute(cctx, cmd)
		printResult(name, time.Since(start), res.ExitCode, res.Stdout, res.Stderr, err)
	}

	runTool := func(name string, args map[string]interface{}, allowEmbedded bool, timeout time.Duration) {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if allowEmbedded {
			cctx = tools.WithUICallbacks(cctx, func(context.Context, tools.QuestionRequest) (tools.QuestionResponse, error) {
				return tools.QuestionResponse{SelectedIndex: 0, SelectedLabel: "Run Anyway"}, nil
			}, nil)
		}
		start := time.Now()
		out, err := tool.Execute(cctx, args, exec)
		printResult(name, time.Since(start), out.ExitCode, out.Content, out.ErrorMessage, err)
	}

	runDirect("baseline", "printf 'whoami='; whoami; printf 'uid='; id -u; printf 'sudo='; command -v sudo || true; printf 'su='; command -v su || true", 10*time.Second)
	runDirect("sudo-n-validate", "(sudo -n -v >/dev/null 2>&1); rc=$?; echo sudo_n_v_rc=$rc", 10*time.Second)
	runTool("become-sudo-root", map[string]interface{}{
		"command":       "printf 'whoami='; whoami; printf 'uid='; id -u",
		"become_method": "sudo",
		"become_user":   "root",
	}, false, 15*time.Second)
	runTool("leading-inline-sudo", map[string]interface{}{
		"command": `sudo sh -lc 'printf "whoami="; whoami; printf "uid="; id -u'`,
	}, false, 15*time.Second)
	runTool("leading-compound-sudo", map[string]interface{}{
		"command": "sudo whoami && whoami",
	}, true, 15*time.Second)
	runTool("embedded-sudo-tee", map[string]interface{}{
		"command": "printf 'embedded-ok\\n' | sudo tee /tmp/infractl_sudo_su_verify >/dev/null",
	}, true, 15*time.Second)
	runTool("become-su-login-user", map[string]interface{}{
		"command":       "printf 'whoami='; whoami; printf 'uid='; id -u",
		"become_method": "su",
		"become_user":   srv.User,
	}, false, 15*time.Second)
	runTool("sudo-invalid-user", map[string]interface{}{
		"command":       "id -un",
		"become_method": "sudo",
		"become_user":   "__infractl_no_such_user__",
	}, false, 10*time.Second)
	runTool("cleanup", map[string]interface{}{
		"command":       "rm -f /tmp/infractl_sudo_su_verify",
		"become_method": "sudo",
		"become_user":   "root",
	}, false, 10*time.Second)
}

func printResult(name string, elapsed time.Duration, exitCode int, stdout, stderr string, err error) {
	fmt.Printf("\n=== %s (%s) ===\n", name, elapsed.Round(time.Millisecond))
	fmt.Printf("exit=%d err=%v\n", exitCode, err)
	if strings.TrimSpace(stdout) != "" {
		fmt.Println("--- output ---")
		fmt.Println(trimForLog(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		fmt.Println("--- stderr/error ---")
		fmt.Println(trimForLog(stderr))
	}
}

func trimForLog(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if len(s) > 1200 {
		return s[:1200] + "\n...<truncated>..."
	}
	return s
}
