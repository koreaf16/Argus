// Package routing
// File: routing_test.go
// Description: Unit tests for RouteUser deterministic routing.

package routing

import (
	"testing"

	"github.com/yourorg/infractl/internal/store"
)

func TestRouteUser(t *testing.T) {
	server := &store.Server{
		ServiceUsers: []store.ServiceUser{
			{Name: "oracle", Access: "sudo_su", Verified: true},
			{Name: "postgres", Access: "sudo_su", Verified: true},
		},
	}

	cases := []struct {
		name     string
		cmd      string
		wantUser string
	}{
		{"sqlplus", "sqlplus / as sysdba", "oracle"},
		{"runInstaller", "/home/oracle/database/runInstaller -silent", "oracle"},
		{"dbca", "dbca -silent -createDatabase", "oracle"},
		{"lsnrctl", "lsnrctl start", "oracle"},
		{"opatch", "/u01/app/oracle/product/19c/OPatch/opatch apply", "oracle"},
		{"unzip oracle archive", "unzip LINUX.X64_193000_db_home.zip -d /u01/app/oracle/product/19.0.0/dbhome_1", "oracle"},
		{"tar oracle archive", "tar -xzf p12345678_190000.tar.gz -C /u01/", "oracle"},
		{"oracle path ref", "mkdir -p /home/oracle/scripts", "oracle"},
		{"oracle home env", "ls $ORACLE_HOME/bin", "oracle"},
		{"psql", "psql -U postgres -d mydb", "postgres"},
		{"pg_ctl", "pg_ctl start -D /var/lib/pgsql/data", "postgres"},
		{"systemctl", "systemctl restart sshd", "root"},
		{"useradd", "useradd oracle", "root"},
		{"firewall-cmd", "firewall-cmd --add-port=1521/tcp", "root"},
		{"ls command", "ls -la /tmp", ""},
		{"cat command", "cat /etc/hosts", ""},
		{"oracle diagnostic readonly", "id oracle && df -h /home/oracle /tmp && cat /etc/os-release | head -5", ""},
		{"oracle path mkdir", "mkdir -p /home/oracle/scripts && chmod 755 /home/oracle/scripts", "oracle"},
		{"oracle cp", "cp /tmp/file.zip /home/oracle/", "oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := RouteUser("shell_exec", map[string]any{"command": tc.cmd}, server)
			if result.AsUser != tc.wantUser {
				t.Errorf("RouteUser(%q) = %q (reason: %s), want %q", tc.cmd, result.AsUser, result.Reason, tc.wantUser)
			}
		})
	}
}

func TestRouteUser_ExplicitOverride(t *testing.T) {
	result := RouteUser("shell_exec", map[string]any{
		"command": "ls /tmp",
		"as_user": "oracle",
	}, nil)
	if result.AsUser != "oracle" {
		t.Errorf("explicit as_user not respected, got %q", result.AsUser)
	}
	if result.Reason != "explicit as_user" {
		t.Errorf("expected reason 'explicit as_user', got %q", result.Reason)
	}
}

func TestRouteUser_NonShellTool(t *testing.T) {
	result := RouteUser("file_read", map[string]any{"path": "/home/oracle/test"}, nil)
	if result.AsUser != "" {
		t.Errorf("non-shell tool should not be routed, got %q", result.AsUser)
	}
}

func TestRouteUser_FileTransferOracleUpload(t *testing.T) {
	result := RouteUser("file_transfer", map[string]any{
		"action":      "upload",
		"remote_path": "/home/oracle/LINUX.X64_193000_db_home.zip",
	}, nil)
	if result.AsUser != "oracle" {
		t.Fatalf("file_transfer oracle upload routed to %q, want oracle", result.AsUser)
	}
	if result.Reason != "oracle file upload path" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}
}

func TestRouteUser_FileWriteOraclePath(t *testing.T) {
	result := RouteUser("file_write", map[string]any{
		"path": "/u01/app/oracle/product/19c/dbhome_1/network/admin/listener.ora",
	}, nil)
	if result.AsUser != "oracle" {
		t.Fatalf("file_write oracle path routed to %q, want oracle", result.AsUser)
	}
}
