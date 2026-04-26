// Package tools
// File: server_add.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"
	"strings"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/serverenv"
	"github.com/yourorg/infractl/internal/store"
)

// ServerAddTool registers a new SSH target after validating connectivity.
type ServerAddTool struct {
	Store    store.ServerStore
	Manager  *executor.Manager
	ToolName string
}

func (t *ServerAddTool) Name() string {
	if strings.TrimSpace(t.ToolName) != "" {
		return t.ToolName
	}
	return "server_add"
}

func (t *ServerAddTool) Description() string {
	return "Register a new SSH server. Tests connectivity before saving. Supports key or password auth."
}

func (t *ServerAddTool) IsReadOnly() bool { return false }
func (t *ServerAddTool) IsEnabled() bool  { return true }

func (t *ServerAddTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Server name (e.g., 'db-server')",
			},
			"host": map[string]interface{}{
				"type":        "string",
				"description": "IP address or hostname",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"description": "SSH port (default: 22)",
			},
			"user": map[string]interface{}{
				"type":        "string",
				"description": "SSH username",
			},
			"auth_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"key", "password"},
				"description": "Authentication method: 'key' for SSH private key, 'password' for password",
			},
			"credential": map[string]interface{}{
				"type":        "string",
				"description": "For auth_type=key: path to private key file. For auth_type=password: the password.",
			},
			"workspace_dir": map[string]interface{}{
				"type":        "string",
				"description": "Remote workspace directory for this SSH account. Default: ~/.infractl/workspace.",
			},
		},
		"required": []string{"name", "host", "user", "auth_type", "credential"},
	}
}

func (t *ServerAddTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	fail := func(format string, args ...interface{}) (ToolOutcome, error) {
		msg := fmt.Sprintf(format, args...)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	name, err := requiredTrimmedString(args, "name")
	if err != nil {
		return fail("Error: %s", err)
	}
	if err := validateServerName(name); err != nil {
		return fail("Error: %s", err)
	}

	host, err := requiredTrimmedString(args, "host")
	if err != nil {
		return fail("Error: %s", err)
	}
	user, err := requiredTrimmedString(args, "user")
	if err != nil {
		return fail("Error: %s", err)
	}
	authType, err := requiredTrimmedString(args, "auth_type")
	if err != nil {
		return fail("Error: %s", err)
	}
	authType = strings.ToLower(authType)
	switch authType {
	case "key", "password":
	default:
		return fail("Error: invalid auth_type %q: must be key or password", authType)
	}
	credential, err := requiredTrimmedString(args, "credential")
	if err != nil {
		return fail("Error: %s", err)
	}

	port := argInt(args, "port", 22)
	workspaceDir, _ := argString(args, "workspace_dir", false)
	workspaceDir = serverenv.RemoteDirOrDefault(strings.TrimSpace(workspaceDir))

	if t.Store == nil {
		return fail("Error: server store is not configured")
	}
	existing, found, err := t.findExistingServer(ctx, name)
	if err != nil {
		return fail("Error: unable to check existing servers: %s", err)
	}
	if found {
		return fail("Error: server already registered: %s", existing.Name)
	}

	if t.Manager == nil {
		return fail("Error: executor manager is nil")
	}

	cfg := &sshconn.Config{
		Host:         host,
		Port:         port,
		User:         user,
		AuthType:     authType,
		WorkspaceDir: workspaceDir,
	}
	if authType == "key" {
		cfg.KeyPath = credential
	} else {
		cfg.Password = credential
	}

	client := sshconn.NewClient(cfg)
	result, runErr := client.Run(ctx, "echo ok")
	if runErr != nil {
		client.Close()
		return fail("SSH connection test failed for %s@%s:%d: %s", user, host, port, runErr)
	}
	if result.ExitCode != 0 {
		client.Close()
		return fail("SSH test command failed (exit %d): %s", result.ExitCode, result.Stderr)
	}

	workspaceCmd := fmt.Sprintf("mkdir -p %s", serverenv.POSIXShellPath(workspaceDir))
	workspaceResult, workspaceErr := client.Run(ctx, workspaceCmd)
	if workspaceErr != nil {
		client.Close()
		return fail("Server directory setup failed for %s: %s", workspaceDir, workspaceErr)
	}
	if workspaceResult.ExitCode != 0 {
		client.Close()
		return fail("Server directory setup failed (exit %d): %s", workspaceResult.ExitCode, workspaceResult.Stderr)
	}

	probeExec := sshconn.NewSSHExecutor(name, client)
	osInfo, _ := executor.DetectOS(ctx, probeExec)

	srv := store.Server{
		Name:         name,
		Host:         host,
		Port:         port,
		User:         user,
		AuthType:     store.AuthType(authType),
		Credential:   credential,
		OS:           osInfo,
		WorkspaceDir: workspaceDir,
	}
	if err := t.Store.Add(ctx, srv); err != nil {
		client.Close()
		return fail("Failed to save server: %s", err)
	}

	sshExec := sshconn.NewSSHExecutor(name, client, osInfo, workspaceDir)
	t.Manager.Register(name, sshExec)

	if strings.TrimSpace(osInfo) == "" {
		return ToolOutcome{Content: fmt.Sprintf("Server '%s' registered successfully (%s@%s:%d, auth: %s, workspace: %s)", name, user, host, port, authType, workspaceDir), Success: true}, nil
	}
	return ToolOutcome{Content: fmt.Sprintf("Server '%s' registered successfully (%s@%s:%d, auth: %s, os: %s, workspace: %s)", name, user, host, port, authType, osInfo, workspaceDir), Success: true}, nil
}

func requiredTrimmedString(args map[string]interface{}, key string) (string, error) {
	value, err := argString(args, key, true)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be blank", key)
	}
	return value, nil
}

func validateServerName(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be blank")
	}
	if strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("invalid name %q: whitespace is not allowed", name)
	}
	switch strings.ToLower(name) {
	case "localhost", "local":
		return fmt.Errorf("reserved local alias %q", name)
	}
	return nil
}

func (t *ServerAddTool) findExistingServer(ctx context.Context, name string) (store.Server, bool, error) {
	if t.Store == nil {
		return store.Server{}, false, nil
	}
	servers, err := t.Store.List(ctx)
	if err != nil {
		return store.Server{}, false, err
	}
	for _, srv := range servers {
		if strings.EqualFold(strings.TrimSpace(srv.Name), name) {
			return srv, true, nil
		}
	}
	return store.Server{}, false, nil
}
