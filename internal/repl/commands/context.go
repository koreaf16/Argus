package commands

import (
	"context"
	"io"

	"github.com/koreaf16/argus/internal/connector"
	"github.com/koreaf16/argus/internal/memdir"
	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/lsp"
	"github.com/koreaf16/argus/internal/services/mcp"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/skills"
	"github.com/koreaf16/argus/internal/state"
)

// ServerFormRequest is the input to an interactive server-add form.
type ServerFormRequest struct {
	// EditAlias is non-empty when editing an existing server.
	EditAlias string
}

// ServerListResult is returned by the interactive server list TUI panel.
type ServerListResult struct {
	Action    string // "closed", "add", "edit"
	EditAlias string // non-empty when Action == "edit"
}

// ServerFormResult is the output produced by the interactive server-add form.
type ServerFormResult struct {
	Entry        workspace.ServerEntry
	Password     string // plaintext; dispatcher decides whether to persist
	SavePassword bool   // user opted-in to DPAPI persistence
}

type CommandContext struct {
	Context      context.Context
	Stdout       io.Writer
	State        *state.AppState
	Registry     *llm.Registry
	Engine       *query.Engine
	ModelPath    string
	SettingsPath string
	WorkDir      string
	Memory       *memdir.Store
	Connector    *connector.Manager
	MCP          *mcp.Manager
	LSP          *lsp.Manager
	Workspace    *workspace.Manager
	Skills       *skills.Registry
	Credentials  *workspace.CredentialStore
	ModelHandler func(args []string) error
	MCPReload    func() error
	// ServerFormPrompt opens the interactive server-registration form.
	// Nil in non-TUI contexts (print mode, pure REPL); callers should fall back to CLI args.
	ServerFormPrompt func(ctx context.Context, req ServerFormRequest) (ServerFormResult, error)
	// ServerListPrompt opens the interactive server list panel.
	// Nil in non-TUI contexts; callers should fall back to text list output.
	ServerListPrompt func(ctx context.Context) (ServerListResult, error)
	// ConnectorSearchPrompt opens the interactive connector search result panel.
	// Nil in non-TUI contexts; callers should fall back to text output.
	ConnectorSearchPrompt func(ctx context.Context, query string, results []connector.ConnectorSpec) (*connector.ConnectorSpec, error)
	// ConnectorInstallPrompt opens the interactive connector env-var install form.
	// Nil in non-TUI contexts; callers should fall back to CLI key=value args.
	ConnectorInstallPrompt func(ctx context.Context, spec connector.ConnectorSpec) (map[string]string, bool, error)
}
