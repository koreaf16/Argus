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

	"github.com/koreaf16/argus/internal/connector"
	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/inventory"
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
		fmt.Fprintln(ctx.Stdout, "/help /exit /clear /model /status /session /plan /memory /mcp /server /skills /commit /diff /review /init /config /keybindings /connector")
		fmt.Fprintln(ctx.Stdout, "Hints:")
		fmt.Fprintln(ctx.Stdout, "/session save|load|list  /memory add|list|search  /mcp list|reload|tools|resources|read  /server list|add|edit|connect|account|use|copy|ls|metrics|tunnel")
		fmt.Fprintln(ctx.Stdout, "/connector search|install|list|remove|info")
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
	case "connector":
		return false, handleConnector(args, ctx)
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
	case "viewthink":
		return false, handleViewThink(args, ctx)
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

func handleConnector(args []string, ctx CommandContext) error {
	if ctx.Connector == nil {
		return fmt.Errorf("connector manager is unavailable")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: /connector <search|install|list|remove|info>")
	}
	switch strings.ToLower(args[0]) {
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("usage: /connector search <query>")
		}
		query := strings.Join(args[1:], " ")
		fmt.Fprintf(ctx.Stdout, "Searching for %q...\n", query)
		results, err := ctx.Connector.Aggregator.Search(ctx.Context, query)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Fprintln(ctx.Stdout, "No connectors found.")
			return nil
		}
		if ctx.ConnectorSearchPrompt != nil {
			// TUI: 모달로 검색 결과 표시, 선택 시 설치 모달 오픈
			spec, err := ctx.ConnectorSearchPrompt(ctx.Context, query, results)
			if err != nil {
				return err
			}
			if spec == nil {
				return nil // 사용자가 Esc로 취소
			}
			return connectorInstallSpec(ctx, *spec, nil)
		}
		// Non-TUI fallback: 텍스트 출력
		for _, r := range results {
			fmt.Fprintf(ctx.Stdout, "- %s (%s) [%s]\n  %s\n", r.Name, r.Runtime, r.Source, r.Description)
		}
		return nil
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: /connector install <name> [KEY=VALUE ...]")
		}
		name := args[1]
		spec, err := ctx.Connector.Aggregator.Info(ctx.Context, name)
		if err != nil {
			return err
		}
		// CLI key=value 인자 파싱 (non-TUI fallback용)
		cliEnv := make(map[string]string)
		for _, arg := range args[2:] {
			if idx := strings.IndexByte(arg, '='); idx > 0 {
				cliEnv[arg[:idx]] = arg[idx+1:]
			}
		}
		return connectorInstallSpec(ctx, *spec, cliEnv)
	case "list":
		installed, err := ctx.Connector.Installer.ListInstalled()
		if err != nil {
			return err
		}
		if len(installed) == 0 {
			fmt.Fprintln(ctx.Stdout, "No connectors installed.")
			return nil
		}
		for _, c := range installed {
			fmt.Fprintf(ctx.Stdout, "- %s (%s)\n", c.Spec.Name, c.Spec.Command)
		}
		return nil
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: /connector remove <name>")
		}
		name := args[1]
		if err := ctx.Connector.Installer.Remove(name); err != nil {
			return err
		}
		if ctx.MCPReload != nil {
			_ = ctx.MCPReload()
		}
		fmt.Fprintf(ctx.Stdout, "Successfully removed %s.\n", name)
		return nil
	case "info":
		if len(args) < 2 {
			return fmt.Errorf("usage: /connector info <name>")
		}
		name := args[1]
		spec, err := ctx.Connector.Aggregator.Info(ctx.Context, name)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(spec, "", "  ")
		fmt.Fprintln(ctx.Stdout, string(b))
		return nil
	default:
		return fmt.Errorf("usage: /connector <search|install|list|remove|info>")
	}
}

// connectorInstallSpec installs a connector spec, using TUI prompts when available.
// cliEnv contains key=value pairs parsed from CLI args (used when TUI is unavailable).
func connectorInstallSpec(ctx CommandContext, spec connector.ConnectorSpec, cliEnv map[string]string) error {
	envAnswers := cliEnv
	if envAnswers == nil {
		envAnswers = make(map[string]string)
	}

	if len(spec.EnvPrompts) > 0 {
		if ctx.ConnectorInstallPrompt != nil {
			answers, cancelled, err := ctx.ConnectorInstallPrompt(ctx.Context, spec)
			if err != nil {
				return err
			}
			if cancelled {
				return nil
			}
			envAnswers = answers
		} else {
			// Non-TUI: 필수 환경변수 누락 검사
			var missing []string
			for _, ep := range spec.EnvPrompts {
				if ep.Required {
					if _, ok := envAnswers[ep.Key]; !ok {
						missing = append(missing, ep.Key)
					}
				}
			}
			if len(missing) > 0 {
				fmt.Fprintf(ctx.Stdout, "Required: %s\n", strings.Join(missing, ", "))
				fmt.Fprintf(ctx.Stdout, "Usage: /connector install %s KEY=VALUE ...\n", spec.Name)
				return fmt.Errorf("missing required environment variables")
			}
		}
	}

	fmt.Fprintf(ctx.Stdout, "Installing %s...\n", spec.Name)
	if err := ctx.Connector.Installer.Install(ctx.Context, spec, envAnswers); err != nil {
		return err
	}
	if ctx.MCPReload != nil {
		_ = ctx.MCPReload()
	}
	fmt.Fprintf(ctx.Stdout, "✓ %s 설치 완료\n", spec.Name)
	return nil
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
	case "account", "target":
		return handleServerAccount(args[1:], ctx)
	case "edit":
		if len(args) < 2 {
			return fmt.Errorf("usage: /server edit <alias>")
		}
		return handleServerEditForm(ctx, args[1])
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
		fmt.Fprintf(ctx.Stdout, "인벤토리 수집 중 (백그라운드)...\n")
		ctx.Workspace.StartInventoryScan(ctx.Context, resolvedAlias, nil)
		return nil
	case "scan":
		scanAlias := ctx.Workspace.ActiveAlias()
		if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
			scanAlias = args[0]
		}
		if scanAlias == workspace.LocalAlias {
			return fmt.Errorf("로컬 워크스페이스는 인벤토리 스캔을 지원하지 않습니다")
		}
		fmt.Fprintf(ctx.Stdout, "인벤토리 수집 중: %s...\n", scanAlias)
		inventorySnap, scanErr := ctx.Workspace.RescanInventory(ctx.Context, scanAlias)
		if scanErr != nil {
			return fmt.Errorf("인벤토리 수집 실패: %w", scanErr)
		}
		fmt.Fprint(ctx.Stdout, inventory.FormatSummaryForPrompt(inventorySnap))
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
		return fmt.Errorf("usage: /server [list|add|account|edit|rm|connect|use|disconnect|status|import|pull|copy|ls|metrics|tunnel]")
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

type serverAccountOptions struct {
	HostAlias    string
	User         string
	Alias        string
	Method       string
	DefaultCWD   string
	Password     string
	SavePassword bool
	Shell        string
	Home         string
	Groups       string
	NoCreateHome bool
}

func handleServerAccount(args []string, ctx CommandContext) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /server account <add|create> <hostAlias> <user> [--alias <alias>] [--method su|sudo] [--cwd <path>]")
	}
	switch strings.ToLower(args[0]) {
	case "add":
		opts, err := parseServerAccountOptions(args[1:], false)
		if err != nil {
			return err
		}
		return addServerAccountTarget(ctx, opts)
	case "create":
		opts, err := parseServerAccountOptions(args[1:], true)
		if err != nil {
			return err
		}
		if err := createRemoteAccount(ctx, opts); err != nil {
			return err
		}
		return addServerAccountTarget(ctx, opts)
	default:
		return fmt.Errorf("usage: /server account <add|create> <hostAlias> <user> [--alias <alias>] [--method su|sudo] [--cwd <path>]")
	}
}

func parseServerAccountOptions(args []string, allowCreateFlags bool) (serverAccountOptions, error) {
	if len(args) < 2 {
		return serverAccountOptions{}, fmt.Errorf("usage: /server account <add|create> <hostAlias> <user> [--alias <alias>] [--method su|sudo] [--cwd <path>]")
	}
	opts := serverAccountOptions{
		HostAlias: strings.ToLower(strings.TrimSpace(args[0])),
		User:      strings.TrimSpace(args[1]),
		Method:    workspace.PrivilegeSU,
		Shell:     "/bin/bash",
	}
	if !isSafeLinuxName(opts.User) {
		return serverAccountOptions{}, fmt.Errorf("invalid Linux user name: %s", opts.User)
	}
	for i := 2; i < len(args); i++ {
		switch strings.ToLower(args[i]) {
		case "--alias":
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("--alias requires a value")
			}
			opts.Alias = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--method":
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("--method requires su or sudo")
			}
			switch strings.ToLower(strings.TrimSpace(args[i+1])) {
			case workspace.PrivilegeSudo:
				opts.Method = workspace.PrivilegeSudo
			case workspace.PrivilegeSU, "":
				opts.Method = workspace.PrivilegeSU
			default:
				return serverAccountOptions{}, fmt.Errorf("--method requires su or sudo")
			}
			i++
		case "--cwd":
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("--cwd requires a value")
			}
			opts.DefaultCWD = args[i+1]
			i++
		case "--password", "--su-password":
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			opts.Password = args[i+1]
			i++
		case "--password-env", "--su-password-env":
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("%s requires an environment variable name", args[i])
			}
			opts.Password = os.Getenv(args[i+1])
			if strings.TrimSpace(opts.Password) == "" {
				return serverAccountOptions{}, fmt.Errorf("environment variable %s is empty", args[i+1])
			}
			i++
		case "--save-password":
			opts.SavePassword = true
		case "--shell":
			if !allowCreateFlags {
				return serverAccountOptions{}, fmt.Errorf("--shell is only valid with /server account create")
			}
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("--shell requires a value")
			}
			opts.Shell = args[i+1]
			i++
		case "--home":
			if !allowCreateFlags {
				return serverAccountOptions{}, fmt.Errorf("--home is only valid with /server account create")
			}
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("--home requires a value")
			}
			opts.Home = args[i+1]
			i++
		case "--groups":
			if !allowCreateFlags {
				return serverAccountOptions{}, fmt.Errorf("--groups is only valid with /server account create")
			}
			if i+1 >= len(args) {
				return serverAccountOptions{}, fmt.Errorf("--groups requires a comma-separated value")
			}
			if !isSafeLinuxGroupList(args[i+1]) {
				return serverAccountOptions{}, fmt.Errorf("invalid group list: %s", args[i+1])
			}
			opts.Groups = args[i+1]
			i++
		case "--no-create-home":
			if !allowCreateFlags {
				return serverAccountOptions{}, fmt.Errorf("--no-create-home is only valid with /server account create")
			}
			opts.NoCreateHome = true
		default:
			return serverAccountOptions{}, fmt.Errorf("unknown account option: %s", args[i])
		}
	}
	if opts.Alias == "" {
		opts.Alias = defaultAccountAlias(opts.HostAlias, opts.User)
	}
	return opts, nil
}

func addServerAccountTarget(ctx CommandContext, opts serverAccountOptions) error {
	parent, ok := ctx.Workspace.Registry().Get(opts.HostAlias)
	if !ok {
		return fmt.Errorf("unknown parent host alias: %s", opts.HostAlias)
	}
	if parent.Kind != workspace.ServerKindSSH {
		return fmt.Errorf("parent alias %s is not an SSH host", opts.HostAlias)
	}
	entry := workspace.ServerEntry{
		Alias:        opts.Alias,
		Kind:         workspace.ServerKindAccount,
		ParentAlias:  parent.Alias,
		User:         opts.User,
		SwitchMethod: opts.Method,
		DefaultCWD:   opts.DefaultCWD,
	}
	if err := ctx.Workspace.Registry().Add(entry); err != nil {
		return err
	}
	if err := ctx.Workspace.Registry().Save(); err != nil {
		return err
	}
	storeAccountTargetPassword(ctx, parent.Alias, opts)
	fmt.Fprintf(ctx.Stdout, "added work account: %s (%s via %s, method=%s)\n", entry.Alias, entry.User, entry.ParentAlias, entry.SwitchMethod)
	return nil
}

func createRemoteAccount(ctx CommandContext, opts serverAccountOptions) error {
	parent, ok := ctx.Workspace.Registry().Get(opts.HostAlias)
	if !ok {
		return fmt.Errorf("unknown parent host alias: %s", opts.HostAlias)
	}
	if parent.Kind != workspace.ServerKindSSH {
		return fmt.Errorf("parent alias %s is not an SSH host", opts.HostAlias)
	}
	var useradd []string
	useradd = append(useradd, "useradd")
	if !opts.NoCreateHome {
		useradd = append(useradd, "-m")
	} else {
		useradd = append(useradd, "-M")
	}
	if strings.TrimSpace(opts.Shell) != "" {
		useradd = append(useradd, "-s", opts.Shell)
	}
	if strings.TrimSpace(opts.Home) != "" {
		useradd = append(useradd, "-d", opts.Home)
	}
	useradd = append(useradd, opts.User)

	var script strings.Builder
	script.WriteString("set -e\n")
	script.WriteString("if ! id -u " + posixQuote(opts.User) + " >/dev/null 2>&1; then\n")
	script.WriteString("  if command -v useradd >/dev/null 2>&1; then\n")
	script.WriteString("    " + shellWords(useradd...) + "\n")
	script.WriteString("  elif command -v adduser >/dev/null 2>&1; then\n")
	script.WriteString("    adduser --disabled-password --gecos '' --shell " + posixQuote(opts.Shell) + " " + posixQuote(opts.User) + "\n")
	script.WriteString("  else\n")
	script.WriteString("    echo 'useradd/adduser not found' >&2; exit 127\n")
	script.WriteString("  fi\n")
	script.WriteString("fi\n")
	if strings.TrimSpace(opts.Groups) != "" {
		script.WriteString("usermod -aG " + posixQuote(opts.Groups) + " " + posixQuote(opts.User) + "\n")
	}
	if opts.Method == workspace.PrivilegeSU && strings.TrimSpace(opts.Password) != "" {
		script.WriteString("printf '%s:%s\\n' " + posixQuote(opts.User) + " " + posixQuote(opts.Password) + " | chpasswd\n")
	}
	cmd := "sudo -S -- sh -c " + posixQuote(script.String())
	res, err := ctx.Workspace.ExecWithOptions(ctx.Context, parent.Alias, cmd, workspace.ExecOptions{
		Shell:           "bash",
		ReuseSession:    workspace.ReuseSessionRequired,
		PrivilegeMethod: workspace.PrivilegeSudo,
	}, nil)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("create account failed: %s", strings.TrimSpace(commandFirstNonEmpty(res.Stderr, res.Stdout)))
	}
	fmt.Fprintf(ctx.Stdout, "created/verified remote account: %s on %s\n", opts.User, parent.Alias)
	return nil
}

func storeAccountTargetPassword(ctx CommandContext, parentAlias string, opts serverAccountOptions) {
	if strings.TrimSpace(opts.Password) == "" {
		return
	}
	if opts.SavePassword && ctx.Credentials != nil {
		if err := ctx.Credentials.SetPasswordForTarget(parentAlias, opts.Method, opts.User, opts.Password); err != nil {
			fmt.Fprintf(ctx.Stdout, "warning: could not save account credential: %v\n", err)
		} else if err := ctx.Credentials.Save(); err != nil {
			fmt.Fprintf(ctx.Stdout, "warning: could not write credential store: %v\n", err)
		}
		return
	}
	ctx.Workspace.SetPasswordForTarget(parentAlias, opts.Method, opts.User, opts.Password)
}

func defaultAccountAlias(hostAlias, user string) string {
	clean := strings.ToLower(strings.TrimSpace(user))
	var b strings.Builder
	for _, r := range clean {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		} else if r == '_' || r == '.' {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return strings.ToLower(strings.TrimSpace(hostAlias)) + "-account"
	}
	return strings.ToLower(strings.TrimSpace(hostAlias)) + "-" + b.String()
}

func isSafeLinuxName(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeLinuxGroupList(s string) bool {
	for _, part := range strings.Split(s, ",") {
		if !isSafeLinuxName(strings.TrimSpace(part)) {
			return false
		}
	}
	return true
}

func shellWords(words ...string) string {
	quoted := make([]string, 0, len(words))
	for _, w := range words {
		quoted = append(quoted, posixQuote(w))
	}
	return strings.Join(quoted, " ")
}

func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func commandFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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

func handleViewThink(args []string, ctx CommandContext) error {
	path := strings.TrimSpace(ctx.SettingsPath)
	if path == "" {
		return fmt.Errorf("settings path is empty")
	}
	
	root, err := loadSettings(path)
	if err != nil {
		return err
	}
	
	currentVal, _ := getNested(root, "ui.view_thinking")
	newState := true
	if currentBool, ok := currentVal.(bool); ok {
		newState = !currentBool
	}
	
	if len(args) > 0 {
		arg := strings.ToLower(args[0])
		if arg == "on" || arg == "true" {
			newState = true
		} else if arg == "off" || arg == "false" {
			newState = false
		} else {
			return fmt.Errorf("usage: /viewthink [on|off]")
		}
	}
	
	setNested(root, "ui.view_thinking", newState)
	
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	
	status := "off"
	if newState {
		status = "on"
	}
	fmt.Fprintf(ctx.Stdout, "viewthink is now %s\n", status)
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
		if st.Kind == workspace.ServerKindAccount && strings.TrimSpace(st.ParentAlias) != "" {
			line += " parent=" + st.ParentAlias
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
