package connector

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMcpConnector_BuildConfig(t *testing.T) {
	conn := NewMcpConnector()

	t.Run("Static Mapping - Postgres", func(t *testing.T) {
		info := ServiceInfo{
			ServiceType: "mcp-postgres",
			Details: map[string]string{
				"env_password": "POSTGRES_PASSWORD",
			},
		}
		creds := Credentials{Password: "secret"}
		config := conn.buildConfig(info, creds)

		if config.Command != "npx" {
			t.Errorf("expected command npx, got %s", config.Command)
		}
		if config.Env["POSTGRES_PASSWORD"] != "secret" {
			t.Errorf("expected password in env, got %v", config.Env)
		}
	})

	t.Run("Direct Command from Details", func(t *testing.T) {
		args, _ := json.Marshal([]string{"run", "server"})
		info := ServiceInfo{
			ServiceType: "mcp-custom",
			Details: map[string]string{
				"mcp_command": "python",
				"mcp_args":    string(args),
				"env_user":    "DB_USER",
			},
		}
		creds := Credentials{Username: "admin"}
		config := conn.buildConfig(info, creds)

		if config.Command != "python" {
			t.Errorf("expected command python, got %s", config.Command)
		}
		if config.Args[0] != "run" {
			t.Errorf("expected arg run, got %v", config.Args)
		}
		if config.Env["DB_USER"] != "admin" {
			t.Errorf("expected user in env, got %v", config.Env)
		}
	})
}

func TestManager_CreateConnector_McpPrefix(t *testing.T) {
	mgr := NewManager(nil, nil)
	
	t.Run("Mcp Prefix", func(t *testing.T) {
		conn, err := mgr.createConnector("mcp-anything")
		if err != nil {
			t.Fatalf("failed to create mcp connector: %v", err)
		}
		if _, ok := conn.(*McpConnector); !ok {
			t.Errorf("expected *McpConnector, got %T", conn)
		}
	})

	t.Run("Native Registered Factory", func(t *testing.T) {
		mgr.RegisterFactory("native", func() Connector {
			return nil // dummy
		})
		conn, err := mgr.createConnector("native")
		if err != nil {
			t.Fatalf("failed to create native connector: %v", err)
		}
		if conn != nil {
			// it should be nil because our dummy factory returns nil
		}
	})
}

type fakeConfirmHandler struct {
	approved bool
}

func (h *fakeConfirmHandler) Confirm(ctx context.Context, title, message string) (bool, error) {
	return h.approved, nil
}

func TestMcpConnector_Confirmation(t *testing.T) {
	conn := NewMcpConnector()
	handler := &fakeConfirmHandler{approved: false}
	conn.SetConfirmationHandler(handler)

	info := ServiceInfo{
		ServerName:  "test-server",
		ServiceType: "mcp-test",
		Details:     map[string]string{"mcp_command": "echo"},
	}
	
	t.Run("User Denied", func(t *testing.T) {
		handler.approved = false
		tools := conn.GenerateTools(info, Credentials{})
		if tools != nil {
			t.Errorf("expected nil tools when user denies confirmation")
		}
	})
}
