package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/session"
	"github.com/koreaf16/argus/internal/types"
)

func Dispatch(line string, ctx CommandContext) (bool, error) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if len(fields) == 0 {
		return false, nil
	}
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "exit", "quit":
		return true, nil
	case "clear":
		fmt.Fprint(ctx.Stdout, "\x1b[2J\x1b[H")
		return false, nil
	case "help":
		fmt.Fprintln(ctx.Stdout, "Commands:")
		fmt.Fprintln(ctx.Stdout, "/help /exit /clear /model /status /session /plan /memory /mcp /server /skills /commit /diff /review /init /config /keybindings")
		fmt.Fprintln(ctx.Stdout, "Hints:")
		fmt.Fprintln(ctx.Stdout, "/session save|load|list  /memory add|list|search  /mcp list|reload|tools|resources|read  /server list|add|connect|use|copy|ls|metrics|tunnel")
		return false, nil
	case "model":
		if ctx.ModelHandler == nil {
			return false, fmt.Errorf("model handler is unavailable")
		}
		return false, ctx.ModelHandler(args)
	case "status":
		model := "unconfigured"
		plan := false
		sessionID := ""
		if ctx.State != nil {
			_, display, _ := ctx.State.ActiveModel()
			if display != "" {
				model = display
			}
			plan = ctx.State.InPlanMode()
			sessionID = ctx.State.SessionID()
		}
		servers := 0
		if ctx.MCP != nil {
			servers = len(ctx.MCP.ServerNames())
		}
		langs := 0
		if ctx.LSP != nil {
			langs = len(ctx.LSP.Languages())
		}
		skills := 0
		if ctx.Skills != nil {
			skills = len(ctx.Skills.List())
		}
		workspaceAlias := "local"
		if ctx.Workspace != nil {
			workspaceAlias = ctx.Workspace.ActiveAlias()
		} else if ctx.State != nil && strings.TrimSpace(ctx.State.ActiveWorkspace()) != "" {
			workspaceAlias = ctx.State.ActiveWorkspace()
		}
		fmt.Fprintf(ctx.Stdout, "model: %s\nplan mode: %t\nsession: %s\nworkspace: %s\nmcp servers: %d\nlsp languages: %d\nskills: %d\n", model, plan, sessionID, workspaceAlias, servers, langs, skills)
		if ctx.MCP != nil {
			names := ctx.MCP.ServerNames()
			if len(names) > 0 {
				fmt.Fprintf(ctx.Stdout, "mcp list: %s\n", strings.Join(names, ", "))
			}
		}
		if ctx.LSP != nil {
			st := ctx.LSP.Status()
			if len(st) > 0 {
				keys := make([]string, 0, len(st))
				for k := range st {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				pairs := make([]string, 0, len(keys))
				for _, k := range keys {
					pairs = append(pairs, fmt.Sprintf("%s=%s", k, st[k]))
				}
				fmt.Fprintf(ctx.Stdout, "lsp status: %s\n", strings.Join(pairs, ", "))
			}
		}
		return false, nil
	case "session":
		return false, handleSession(args, ctx)
	case "plan":
		return false, handlePlan(args, ctx)
	case "memory":
		return false, handleMemory(args, ctx)
	case "mcp":
		return false, handleMCP(args, ctx)
	case "server":
		return false, handleServer(args, ctx)
	case "skills":
		return false, handleSkills(args, ctx)
	case "commit":
		return false, handleCommit(args, ctx)
	case "diff":
		return false, handleDiff(args, ctx)
	case "review":
		return false, handleReview(ctx)
	case "init":
		return false, handleInit(ctx)
	case "config":
		return false, handleConfig(args, ctx)
	case "keybindings":
		fmt.Fprintln(ctx.Stdout, "keybindings: default readline bindings")
		return false, nil
	default:
		return false, fmt.Errorf("unknown command: /%s", cmd)
	}
}

func handleSession(args []string, ctx CommandContext) error {
	if ctx.Memory == nil {
		return fmt.Errorf("session store is unavailable")
	}
	if len(args) < 1 {
		return fmt.Errorf("usage: /session <save|load|list> [name]")
	}
	switch strings.ToLower(args[0]) {
	case "list":
		names, err := ctx.Memory.ListSessions()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			fmt.Fprintln(ctx.Stdout, "no sessions")
			return nil
		}
		for _, name := range names {
			fmt.Fprintln(ctx.Stdout, name)
		}
		return nil
	case "save":
		if len(args) < 2 {
			return fmt.Errorf("usage: /session save <name>")
		}
		var messages []llm.Message
		if ctx.Engine != nil {
			messages = ctx.Engine.Messages()
		}
		snap := session.Snapshot{
			SavedAt:  time.Now().UTC(),
			Messages: messages,
		}
		if err := ctx.Memory.SaveSession(args[1], snap); err != nil {
			return err
		}
		if ctx.State != nil {
			ctx.State.SetSessionID(args[1])
		}
		fmt.Fprintf(ctx.Stdout, "session saved: %s (%d messages)\n", args[1], len(messages))
		return nil
	case "load":
		if len(args) < 2 {
			return fmt.Errorf("usage: /session load <name>")
		}
		var snap session.Snapshot
		if err := ctx.Memory.LoadSession(args[1], &snap); err != nil {
			return err
		}
		if ctx.Engine != nil {
			ctx.Engine.ReplaceMessages(snap.Messages)
		}
		if ctx.State != nil {
			ctx.State.SetSessionID(args[1])
		}
		fmt.Fprintf(ctx.Stdout, "session loaded: %s (%d messages)\n", args[1], len(snap.Messages))
		return nil
	default:
		return fmt.Errorf("usage: /session <save|load|list> [name]")
	}
}

func handlePlan(args []string, ctx CommandContext) error {
	if ctx.State == nil {
		return nil
	}
	if len(args) == 0 {
		if ctx.State.InPlanMode() {
			fmt.Fprintln(ctx.Stdout, "plan mode: on")
		} else {
			fmt.Fprintln(ctx.Stdout, "plan mode: off")
		}
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "on":
		ctx.State.SetMode("plan")
		ctx.State.SetPermissionMode(types.PermissionModePlan)
		fmt.Fprintln(ctx.Stdout, "plan mode: on")
	case "off":
		ctx.State.SetMode("normal")
		if ctx.State.GetPermissionMode() == types.PermissionModePlan {
			ctx.State.SetPermissionMode(types.PermissionModeDefault)
		}
		fmt.Fprintln(ctx.Stdout, "plan mode: off")
	default:
		return fmt.Errorf("usage: /plan [on|off]")
	}
	return nil
}

func handleMemory(args []string, ctx CommandContext) error {
	if ctx.Memory == nil {
		return fmt.Errorf("memory store is unavailable")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: /memory <add|list|search> ...")
	}
	switch strings.ToLower(args[0]) {
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("usage: /memory add <text>")
		}
		rec, err := ctx.Memory.AddMemory(strings.Join(args[1:], " "), nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "memory added: %s\n", rec.ID)
		return nil
	case "list":
		items, err := ctx.Memory.ListMemories()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(ctx.Stdout, "no memories")
			return nil
		}
		for _, item := range items {
			fmt.Fprintf(ctx.Stdout, "- %s %s\n", item.ID, item.Text)
		}
		return nil
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("usage: /memory search <query>")
		}
		items, err := ctx.Memory.FindRelevantMemories(strings.Join(args[1:], " "), 10)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Fprintf(ctx.Stdout, "- %s %s\n", item.ID, item.Text)
		}
		return nil
	default:
		return fmt.Errorf("usage: /memory <add|list|search> ...")
	}
}

func handleMCP(args []string, ctx CommandContext) error {
	if ctx.MCP == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	if len(args) == 0 || strings.ToLower(args[0]) == "list" {
		names := ctx.MCP.ServerNames()
		if len(names) == 0 {
			fmt.Fprintln(ctx.Stdout, "no mcp servers configured")
			return nil
		}
		for _, name := range names {
			fmt.Fprintln(ctx.Stdout, name)
		}
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "reload":
		if ctx.MCPReload != nil {
			if err := ctx.MCPReload(); err != nil {
				return err
			}
		} else if err := ctx.MCP.Load(); err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "reloaded mcp config: %s\n", ctx.MCP.Path())
		return nil
	case "tools":
		if len(args) < 2 {
			return fmt.Errorf("usage: /mcp tools <server>")
		}
		tools := ctx.MCP.ListTools(args[1])
		fmt.Fprintf(ctx.Stdout, "%s tools: %v\n", args[1], tools)
		return nil
	case "resources":
		if len(args) < 2 {
			return fmt.Errorf("usage: /mcp resources <server>")
		}
		resources := ctx.MCP.ListResources(args[1])
		fmt.Fprintf(ctx.Stdout, "%s resources: %v\n", args[1], resources)
		return nil
	case "read":
		if len(args) < 3 {
			return fmt.Errorf("usage: /mcp read <server> <uri>")
		}
		content, err := ctx.MCP.ReadResource(args[1], strings.Join(args[2:], " "))
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.Stdout, content)
		return nil
	default:
		return fmt.Errorf("usage: /mcp [list|reload|tools <server>|resources <server>|read <server> <uri>]")
	}
}

func handleServer(args []string, ctx CommandContext) error {
	if ctx.Workspace == nil {
		return fmt.Errorf("workspace manager is unavailable")
	}
	if len(args) == 0 {
		if ctx.ServerListPrompt != nil {
			res, err := ctx.ServerListPrompt(ctx.Context)
			if err != nil {
				return err
			}
			switch res.Action {
			case "add":
				return handleServerAddForm(ctx)
			case "edit":
				return handleServerEditForm(ctx, res.EditAlias)
			}
			return nil
		}
		return printServerList(ctx)
	}
	if strings.EqualFold(args[0], "list") {
		return printServerList(ctx)
	}

	switch strings.ToLower(args[0]) {
	case "add":
		if len(args) < 2 {
			return handleServerAddForm(ctx)
		}
		return handleServerAdd(args[1:], ctx)
	case "rm", "remove", "del":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server rm <alias>")
		}
		if err := ctx.Workspace.Registry().Remove(args[1]); err != nil {
			return err
		}
		if err := ctx.Workspace.Registry().Save(); err != nil {
			return err
		}
		_ = ctx.Workspace.Disconnect(args[1])
		if ctx.Credentials != nil {
			ctx.Credentials.Delete(args[1])
			_ = ctx.Credentials.Save()
		}
		syncActiveWorkspaceState(ctx)
		fmt.Fprintf(ctx.Stdout, "removed server: %s\n", args[1])
		return nil
	case "connect":
		alias := ""
		if len(args) >= 2 {
			alias = args[1]
		}
		if strings.TrimSpace(alias) == "" {
			alias = ctx.Workspace.ActiveAlias()
		}
		resolvedAlias, err := ctx.Workspace.ConnectAndActivate(alias)
		if err != nil {
			return err
		}
		syncActiveWorkspaceState(ctx)
		fmt.Fprintf(ctx.Stdout, "connected: %s (active workspace switched)\n", resolvedAlias)
		fmt.Fprintf(ctx.Stdout, "inspecting environment...\n")
		snap, inspectErr := ctx.Workspace.RunInspect(ctx.Context, resolvedAlias)
		if inspectErr != nil {
			fmt.Fprintf(ctx.Stdout, "inspect failed: %v\n", inspectErr)
		} else {
			fmt.Fprintf(ctx.Stdout, workspace.FormatInspectSummary(snap))
		}
		return nil
	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server use <alias>")
		}
		if err := ctx.Workspace.SetActive(args[1]); err != nil {
			return err
		}
		syncActiveWorkspaceState(ctx)
		fmt.Fprintf(ctx.Stdout, "active workspace: %s\n", ctx.Workspace.ActiveAlias())
		return nil
	case "disconnect":
		if len(args) < 2 {
			alias := ctx.Workspace.ActiveAlias()
			if alias == workspace.LocalAlias {
				return nil
			}
			if err := ctx.Workspace.Disconnect(alias); err != nil {
				return err
			}
			_ = ctx.Workspace.SetActive(workspace.LocalAlias)
			syncActiveWorkspaceState(ctx)
			fmt.Fprintf(ctx.Stdout, "disconnected: %s\n", alias)
			return nil
		}
		if strings.EqualFold(args[1], "all") {
			if err := ctx.Workspace.DisconnectAll(); err != nil {
				return err
			}
			_ = ctx.Workspace.SetActive(workspace.LocalAlias)
			syncActiveWorkspaceState(ctx)
			fmt.Fprintln(ctx.Stdout, "disconnected all remote sessions")
			return nil
		}
		wasActive := strings.EqualFold(ctx.Workspace.ActiveAlias(), args[1])
		if err := ctx.Workspace.Disconnect(args[1]); err != nil {
			return err
		}
		if wasActive {
			_ = ctx.Workspace.SetActive(workspace.LocalAlias)
		}
		syncActiveWorkspaceState(ctx)
		fmt.Fprintf(ctx.Stdout, "disconnected: %s\n", args[1])
		return nil
	case "status":
		return handleServer([]string{"list"}, ctx)
	case "import":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server import <localPath> [remotePath]")
		}
		dstAlias := ctx.Workspace.ActiveAlias()
		if dstAlias == workspace.LocalAlias {
			return fmt.Errorf("active workspace is local; use /server use <alias> first")
		}
		srcPath := args[1]
		dstPath := ""
		if len(args) >= 3 {
			dstPath = args[2]
		}
		if strings.TrimSpace(dstPath) == "" {
			dstPath = filepath.Base(srcPath)
		}
		if err := ctx.Workspace.CopyFile(ctx.Context, workspace.LocalAlias, srcPath, dstAlias, dstPath, true); err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "imported %s -> %s:%s\n", srcPath, dstAlias, dstPath)
		return nil
	case "pull":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server pull <remotePath> [localPath]")
		}
		srcAlias := ctx.Workspace.ActiveAlias()
		if srcAlias == workspace.LocalAlias {
			return fmt.Errorf("active workspace is local; use /server use <alias> first")
		}
		srcPath := args[1]
		dstPath := ""
		if len(args) >= 3 {
			dstPath = args[2]
		}
		if strings.TrimSpace(dstPath) == "" {
			dstPath = filepath.Base(srcPath)
		}
		if err := ctx.Workspace.CopyFile(ctx.Context, srcAlias, srcPath, workspace.LocalAlias, dstPath, true); err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "pulled %s:%s -> %s\n", srcAlias, srcPath, dstPath)
		return nil
	case "copy":
		if len(args) < 3 {
			return fmt.Errorf("usage: /server copy <srcAlias>:<srcPath> <dstAlias>:<dstPath>")
		}
		srcAlias, srcPath, err := workspace.ParseEndpointPath(args[1], ctx.Workspace.ActiveAlias())
		if err != nil {
			return err
		}
		dstAlias, dstPath, err := workspace.ParseEndpointPath(args[2], ctx.Workspace.ActiveAlias())
		if err != nil {
			return err
		}
		if err := ctx.Workspace.CopyFile(ctx.Context, srcAlias, srcPath, dstAlias, dstPath, true); err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "copied %s:%s -> %s:%s\n", srcAlias, srcPath, dstAlias, dstPath)
		return nil
	case "ls":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server ls <alias>:<path> [--recursive] [--depth <n>]")
		}
		alias, targetPath, err := workspace.ParseEndpointPath(args[1], ctx.Workspace.ActiveAlias())
		if err != nil {
			return err
		}
		recursive := false
		depth := 0
		for i := 2; i < len(args); i++ {
			switch strings.ToLower(args[i]) {
			case "--recursive", "-r":
				recursive = true
			case "--depth":
				if i+1 >= len(args) {
					return fmt.Errorf("--depth requires a value")
				}
				n, convErr := strconv.Atoi(args[i+1])
				if convErr != nil || n < 0 {
					return fmt.Errorf("invalid depth: %s", args[i+1])
				}
				depth = n
				i++
			default:
				return fmt.Errorf("unknown ls option: %s", args[i])
			}
		}
		items, err := ctx.Workspace.ListDir(ctx.Context, alias, targetPath, recursive, depth)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(ctx.Stdout, "no entries")
			return nil
		}
		for _, item := range items {
			kind := "f"
			if item.IsDir {
				kind = "d"
			}
			fmt.Fprintf(ctx.Stdout, "%s %12d %s %s\n", kind, item.Size, item.ModTime.Format(time.RFC3339), item.Path)
		}
		return nil
	case "metrics":
		alias := ctx.Workspace.ActiveAlias()
		if len(args) >= 2 && strings.TrimSpace(args[1]) != "" {
			alias = strings.TrimSpace(args[1])
		}
		snapshot, err := ctx.Workspace.MetricsSnapshot(ctx.Context, alias)
		if err != nil {
			return err
		}
		raw, _ := json.MarshalIndent(snapshot, "", "  ")
		fmt.Fprintln(ctx.Stdout, string(raw))
		return nil
	case "tunnel":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server tunnel <open|close|list> ...")
		}
		alias := ctx.Workspace.ActiveAlias()
		if alias == workspace.LocalAlias {
			return fmt.Errorf("active workspace is local; use /server use <alias> first")
		}
		switch strings.ToLower(args[1]) {
		case "open":
			if len(args) < 3 {
				return fmt.Errorf("usage: /server tunnel open <remoteAddr> [localAddr]")
			}
			remoteAddr := args[2]
			localAddr := ""
			if len(args) >= 4 {
				localAddr = args[3]
			}
			info, err := ctx.Workspace.OpenTunnel(ctx.Context, alias, localAddr, remoteAddr)
			if err != nil {
				return err
			}
			fmt.Fprintf(ctx.Stdout, "tunnel opened: id=%s local=%s remote=%s\n", info.ID, info.LocalAddr, info.RemoteAddr)
			return nil
		case "close":
			if len(args) < 3 {
				return fmt.Errorf("usage: /server tunnel close <tunnelID>")
			}
			if err := ctx.Workspace.CloseTunnel(alias, args[2]); err != nil {
				return err
			}
			fmt.Fprintf(ctx.Stdout, "tunnel closed: %s\n", args[2])
			return nil
		case "list":
			items, err := ctx.Workspace.ListTunnels(alias)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(ctx.Stdout, "no active tunnels")
				return nil
			}
			for _, item := range items {
				fmt.Fprintf(ctx.Stdout, "%s %s -> %s\n", item.ID, item.LocalAddr, item.RemoteAddr)
			}
			return nil
		default:
			return fmt.Errorf("usage: /server tunnel <open|close|list> ...")
		}
	default:
		return fmt.Errorf("usage: /server [list|add|rm|connect|use|disconnect|status|import|pull|copy|ls|metrics|tunnel]")
	}
}

func syncActiveWorkspaceState(ctx CommandContext) {
	if ctx.State == nil || ctx.Workspace == nil {
		return
	}
	active := strings.TrimSpace(ctx.Workspace.ActiveAlias())
	if active == "" {
		active = workspace.LocalAlias
	}
	ctx.State.SetActiveWorkspace(active)
}

func handleServerAdd(args []string, ctx CommandContext) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: /server add <alias> <user>@<host> [--port <port>] [--cwd <path>] [--identity <file>] [--agent] [--password]")
	}
	entry := workspace.ServerEntry{
		Alias: args[0],
		Kind:  workspace.ServerKindSSH,
		Port:  22,
		Auth: workspace.ServerAuth{
			UseAgent:      true,
			AllowPassword: true,
		},
	}
	parts := strings.SplitN(args[1], "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("endpoint must be in user@host format")
	}
	entry.User = strings.TrimSpace(parts[0])
	entry.Host = strings.TrimSpace(parts[1])

	for i := 2; i < len(args); i++ {
		switch strings.ToLower(args[i]) {
		case "--port":
			if i+1 >= len(args) {
				return fmt.Errorf("--port requires a value")
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("invalid port: %w", err)
			}
			entry.Port = port
			i++
		case "--cwd":
			if i+1 >= len(args) {
				return fmt.Errorf("--cwd requires a value")
			}
			entry.DefaultCWD = args[i+1]
			i++
		case "--identity":
			if i+1 >= len(args) {
				return fmt.Errorf("--identity requires a value")
			}
			entry.Auth.IdentityFile = args[i+1]
			i++
		case "--agent":
			entry.Auth.UseAgent = true
		case "--password":
			entry.Auth.AllowPassword = true
		default:
			return fmt.Errorf("unknown add option: %s", args[i])
		}
	}

	if err := ctx.Workspace.Registry().Add(entry); err != nil {
		return err
	}
	if err := ctx.Workspace.Registry().Save(); err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "added server: %s (%s@%s:%d)\n", entry.Alias, entry.User, entry.Host, entry.Port)
	return nil
}

func handleSkills(args []string, ctx CommandContext) error {
	if ctx.Skills == nil {
		return fmt.Errorf("skills registry is unavailable")
	}
	if len(args) == 0 || strings.ToLower(args[0]) == "list" {
		for _, skill := range ctx.Skills.List() {
			meta, _ := ctx.Skills.Describe(skill)
			if strings.TrimSpace(meta.Description) == "" {
				fmt.Fprintln(ctx.Stdout, skill)
				continue
			}
			fmt.Fprintf(ctx.Stdout, "%s - %s\n", skill, meta.Description)
		}
		return nil
	}
	if strings.ToLower(args[0]) == "run" {
		if len(args) < 2 {
			return fmt.Errorf("usage: /skills run <name> [args...]")
		}
		out, err := ctx.Skills.Run(args[1], args[2:])
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.Stdout, out)
		return nil
	}
	return fmt.Errorf("usage: /skills [list|run <name> [args...]]")
}

func handleInit(ctx CommandContext) error {
	configDir := constants.ConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	type seedFile struct {
		path    string
		content string
	}
	files := []seedFile{
		{
			path: constants.SettingsPath(),
			content: `{
  "activeModel": "",
  "permissions": {
    "defaultMode": "ask",
    "allow": [],
    "deny": []
  },
  "web": {
    "searxng": {
      "base_url": "http://192.168.0.3:30080/search",
      "max_results": 10
    },
    "crawl4ai": {
      "base_url": "http://192.168.0.3:30090",
      "timeout_ms": 60000
    }
  },
  "ui": {
    "theme": "argus_ui_demo",
    "variant": "argus-signature",
    "motion": {
      "enabled": true,
      "level": "restrained",
      "tick_ms": 100,
      "reduced": false,
      "signature": true
    },
    "streaming": {
      "mode": "line-stable",
      "hide_unstable_markdown_tail": true,
      "flush_plain_text_partial": true,
      "render_code_blocks_stable": true
    }
  }
}
`,
		},
		{
			path: constants.ModelsPath(),
			content: `{
  "active": "",
  "servers": [],
  "userModels": []
}
`,
		},
		{
			path: filepath.Join(constants.ConfigDir(), "mcp.json"),
			content: `{
  "servers": []
}
`,
		},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o600); err != nil {
			return err
		}
	}
	fmt.Fprintf(ctx.Stdout, "initialized %s\n", configDir)
	return nil
}

func handleConfig(args []string, ctx CommandContext) error {
	path := strings.TrimSpace(ctx.SettingsPath)
	if path == "" {
		return fmt.Errorf("settings path is empty")
	}
	if len(args) == 0 || strings.EqualFold(args[0], "show") {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.Stdout, string(data))
		return nil
	}
	if strings.EqualFold(args[0], "get") {
		if len(args) < 2 {
			return fmt.Errorf("usage: /config get <key>")
		}
		root, err := loadSettings(path)
		if err != nil {
			return err
		}
		val, ok := getNested(root, args[1])
		if !ok {
			return fmt.Errorf("config key not found: %s", args[1])
		}
		out, _ := json.MarshalIndent(val, "", "  ")
		fmt.Fprintln(ctx.Stdout, string(out))
		return nil
	}
	if strings.EqualFold(args[0], "set") {
		if len(args) < 3 {
			return fmt.Errorf("usage: /config set <key> <json-value>")
		}
		root, err := loadSettings(path)
		if err != nil {
			return err
		}
		rawVal := strings.Join(args[2:], " ")
		var value any
		if err := json.Unmarshal([]byte(rawVal), &value); err != nil {
			value = rawVal
		}
		setNested(root, args[1], value)
		data, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "config updated: %s\n", args[1])
		return nil
	}
	return fmt.Errorf("usage: /config [show|get <key>|set <key> <json-value>]")
}

func loadSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root == nil {
		root = make(map[string]any)
	}
	return root, nil
}

func getNested(root map[string]any, dotted string) (any, bool) {
	parts := strings.Split(dotted, ".")
	var cur any = root
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func setNested(root map[string]any, dotted string, value any) {
	parts := strings.Split(dotted, ".")
	cur := root
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = value
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[p] = next
		}
		cur = next
	}
}

func handleCommit(args []string, ctx CommandContext) error {
	if err := ensureGitRepo(ctx.WorkDir, "commit"); err != nil {
		return err
	}
	if len(args) == 0 {
		out, err := gitOutput(ctx.WorkDir, "status", "--short")
		if err != nil {
			return err
		}
		out = strings.TrimSpace(out)
		if out == "" {
			fmt.Fprintln(ctx.Stdout, "working tree is clean; nothing to commit")
			return nil
		}
		fmt.Fprintln(ctx.Stdout, "staged/unstaged summary:")
		fmt.Fprintln(ctx.Stdout, out)
		fmt.Fprintln(ctx.Stdout, "usage: /commit <message>")
		return nil
	}
	msg := strings.Join(args, " ")
	out, err := gitOutput(ctx.WorkDir, "commit", "-m", msg)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, strings.TrimSpace(out))
	return nil
}

func handleDiff(args []string, ctx CommandContext) error {
	if err := ensureGitRepo(ctx.WorkDir, "diff"); err != nil {
		return err
	}
	gitArgs := []string{"diff", "--staged"}
	if len(args) > 0 && strings.EqualFold(args[0], "working") {
		gitArgs = []string{"diff"}
	}
	out, err := gitOutput(ctx.WorkDir, gitArgs...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		fmt.Fprintln(ctx.Stdout, "no diff")
		return nil
	}
	fmt.Fprintln(ctx.Stdout, out)
	return nil
}

func handleReview(ctx CommandContext) error {
	if err := ensureGitRepo(ctx.WorkDir, "review"); err != nil {
		return err
	}
	status, err := gitOutput(ctx.WorkDir, "status", "--short")
	if err != nil {
		return err
	}
	diff, err := gitOutput(ctx.WorkDir, "diff", "--staged", "--stat")
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, "review summary")
	if strings.TrimSpace(status) == "" {
		fmt.Fprintln(ctx.Stdout, "- working tree: clean")
	} else {
		fmt.Fprintln(ctx.Stdout, "- working tree:")
		fmt.Fprintln(ctx.Stdout, strings.TrimSpace(status))
	}
	if strings.TrimSpace(diff) == "" {
		fmt.Fprintln(ctx.Stdout, "- staged diff: none")
	} else {
		fmt.Fprintln(ctx.Stdout, "- staged diff stats:")
		fmt.Fprintln(ctx.Stdout, strings.TrimSpace(diff))
	}
	return nil
}

func ensureGitRepo(workDir, command string) error {
	out, err := gitOutput(workDir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("/%s unavailable: not a git repository (%s)", command, strings.TrimSpace(err.Error()))
	}
	if strings.TrimSpace(out) != "true" {
		return fmt.Errorf("/%s unavailable: not a git repository", command)
	}
	return nil
}

func printServerList(ctx CommandContext) error {
	for _, st := range ctx.Workspace.Status() {
		active := " "
		if st.Active {
			active = "*"
		}
		connected := "disconnected"
		if st.Connected {
			connected = "connected"
		}
		line := fmt.Sprintf("%s %-12s %-5s %s", active, st.Alias, string(st.Kind), connected)
		if strings.TrimSpace(st.CurrentCWD) != "" {
			line += " cwd=" + st.CurrentCWD
		}
		if strings.TrimSpace(st.User) != "" {
			line += " user=" + st.User
		}
		fmt.Fprintln(ctx.Stdout, line)
	}
	return nil
}

func gitOutput(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = filepath.Clean(workDir)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
