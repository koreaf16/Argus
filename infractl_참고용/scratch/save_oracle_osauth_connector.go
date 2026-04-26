//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/connector/oracle"
	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dbPath := filepath.Join("bin", ".infractl", "infractl.db")
	serverName := "oracle"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		dbPath = os.Args[1]
	}
	if len(os.Args) > 2 && strings.TrimSpace(os.Args[2]) != "" {
		serverName = os.Args[2]
	}
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}

	st, err := store.NewSQLiteStore(ctx, absDB)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	servers, err := st.List(ctx)
	if err != nil {
		log.Fatalf("list servers: %v", err)
	}
	var srv *store.Server
	for i := range servers {
		if strings.EqualFold(servers[i].Name, serverName) {
			cp := servers[i]
			srv = &cp
			break
		}
	}
	if srv == nil {
		log.Fatalf("server %q not found", serverName)
	}

	cfg := &sshconn.Config{
		Host:         srv.Host,
		Port:         srv.Port,
		User:         srv.User,
		AuthType:     string(srv.AuthType),
		WorkspaceDir: srv.WorkspaceDir,
	}
	if srv.AuthType == store.AuthTypeKey {
		cfg.KeyPath = srv.Credential
	} else {
		cfg.Password = srv.Credential
	}
	sshExec := sshconn.NewSSHExecutor(srv.Name, sshconn.NewClient(cfg), srv.OS, srv.WorkspaceDir)
	defer sshExec.Close()

	registry := tools.NewRegistry()
	mgr := connector.NewManager(registry, st)
	mgr.RegisterFactory("oracle", func() connector.Connector { return oracle.New() })

	info := connector.ServiceInfo{
		ServerName:  srv.Name,
		ServiceType: "oracle",
		Name:        "AI26",
		SubInstance: "AI_DB",
		Port:        1521,
		Details:     map[string]string{"host": srv.Host, "cdb": "yes"},
	}
	if home := discoverOracleHome(ctx, sshExec, info.Name); home != "" {
		info.Details["oracle_home"] = home
	}
	creds := connector.Credentials{Username: "/", Role: "sysdba", OSAuth: true}
	if err := mgr.Activate(ctx, info, creds, connector.SavePermanent, sshExec); err != nil {
		log.Fatalf("activate/persist oracle connector: %v", err)
	}
	fmt.Printf("saved OS-auth Oracle connector: db=%s server=%s service=%s/%s tools=%v details=%v\n",
		absDB, info.ServerName, info.Name, info.SubInstance, registryToolNames(registry), info.Details)
}

func discoverOracleHome(ctx context.Context, exec executor.Executor, sid string) string {
	cmd := fmt.Sprintf("pid=$(pgrep -fo 'ora_pmon_%s' || true); if [ -n \"$pid\" ]; then readlink -f /proc/$pid/exe | sed 's#/bin/oracle$##'; fi", sid)
	res, err := exec.Execute(ctx, cmd)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

func registryToolNames(reg *tools.Registry) []string {
	var names []string
	for _, tool := range reg.List() {
		names = append(names, tool.Name())
	}
	return names
}
