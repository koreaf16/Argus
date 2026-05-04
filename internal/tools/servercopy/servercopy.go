package servercopy

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerCopyTool struct{}

type copyRequest struct {
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	SrcServer string `json:"src_server"`
	SrcRole   string `json:"src_role"`
	SrcPath   string `json:"src_path"`
	DstServer string `json:"dst_server"`
	DstRole   string `json:"dst_role"`
	DstPath   string `json:"dst_path"`
	Role      string `json:"role"`
	Channel   string `json:"channel"`
	Overwrite *bool  `json:"overwrite"`
}

func NewServerCopyTool() *ServerCopyTool {
	return &ServerCopyTool{}
}

func (t *ServerCopyTool) Name() string {
	return "server_copy"
}

func (t *ServerCopyTool) Description(ctx tool.Context) string {
	return "Copy a file between registered workspaces, including local and SSH servers. Use this for local-to-remote or remote-to-local file transfer."
}

func (t *ServerCopyTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"src":        map[string]any{"type": "string", "description": "Source endpoint path. Use alias:path, for example local:C:\\\\Users\\\\me\\\\file.zip or sandbox-server:/tmp/file.zip."},
			"dst":        map[string]any{"type": "string", "description": "Destination endpoint path. Use alias:path, for example local:C:\\\\Users\\\\me\\\\file.zip or sandbox-server:/tmp/file.zip."},
			"src_server": map[string]any{"type": "string", "description": "Source workspace alias, for example local or sandbox-server."},
			"src_role":   map[string]any{"type": "string", "description": "Source workspace role."},
			"src_path":   map[string]any{"type": "string", "description": "Source file path when src_server is provided separately."},
			"dst_server": map[string]any{"type": "string", "description": "Destination workspace alias, for example local or sandbox-server."},
			"dst_role":   map[string]any{"type": "string", "description": "Destination workspace role."},
			"dst_path":   map[string]any{"type": "string", "description": "Destination file path when dst_server is provided separately."},
			"role":       map[string]any{"type": "string", "description": "Transfer workspace role used as a default endpoint when configured."},
			"channel":    map[string]any{"type": "string", "description": "Transfer workspace channel used as a default endpoint when configured."},
			"overwrite":  map[string]any{"type": "boolean", "description": "Whether to overwrite the destination file. Defaults to true."},
		},
	}
}

func (t *ServerCopyTool) IsReadOnly() bool {
	return false
}

func (t *ServerCopyTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ServerCopyTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 64)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("workspace manager is not available")
	}

	var req copyRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		if err := applyCopyRoleDefaults(ctx, &req); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if err := requireExplicitCopyEndpoints(ctx, req); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		overwrite := true
		if req.Overwrite != nil {
			overwrite = *req.Overwrite
		}

		srcRaw := req.Src
		if srcRaw == "" {
			srcRaw = req.SrcPath
		}
		srcEP, err := workspace.ParseEndpointPathV2(ctx.Workspace, srcRaw, req.SrcServer)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if req.SrcPath != "" && req.Src != "" {
			srcEP.RawPath = req.SrcPath
		}

		dstRaw := req.Dst
		if dstRaw == "" {
			dstRaw = req.DstPath
		}
		dstEP, err := workspace.ParseEndpointPathV2(ctx.Workspace, dstRaw, req.DstServer)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if req.DstPath != "" && req.Dst != "" {
			dstEP.RawPath = req.DstPath
		}

		if srcEP.Alias == "" || srcEP.RawPath == "" || dstEP.Alias == "" || dstEP.RawPath == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("source and destination are required; provide src/dst or src_server/src_path and dst_server/dst_path"))
			return
		}

		events <- tool.NewOutputEvent(fmt.Sprintf("copying %s to %s...", srcEP.RawPath, dstEP.RawPath))

		totalBytes := lookupFileSize(ctx, srcEP)

		lastEmit := time.Now()
		err = ctx.Workspace.CopyFileWithProgress(ctx.Context, srcEP.Alias, srcEP.RawPath, dstEP.Alias, dstEP.RawPath, overwrite, func(copied int64) {
			now := time.Now()
			if now.Sub(lastEmit) < 200*time.Millisecond && copied < totalBytes {
				return
			}
			lastEmit = now

			percent := float64(0)
			if totalBytes > 0 {
				percent = float64(copied) / float64(totalBytes) * 100
			}
			msg := fmt.Sprintf("PROGRESS:%d:%d:%.1f", copied, totalBytes, percent)
			events <- tool.NewChunkEvent(msg)
		})

		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		events <- tool.NewOutputEvent(fmt.Sprintf("copied %s to %s", srcEP.RawPath, dstEP.RawPath))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerCopyTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	var req copyRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if err := applyCopyRoleDefaults(ctx, &req); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if err := requireExplicitCopyEndpoints(ctx, req); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	return types.PermissionResult{Behavior: types.BehaviorAllow}, nil
}

func applyCopyRoleDefaults(ctx tool.Context, req *copyRequest) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.Role) != "" || strings.TrimSpace(req.Channel) != "" {
		roleCtx, err := tool.ResolveExecutionRole(ctx, "", req.Role, req.Channel, "server_copy")
		if err != nil {
			return err
		}
		if err := tool.ValidateRoleMutation(roleCtx, "server_copy", "", false); err != nil {
			return err
		}
	}
	if strings.TrimSpace(req.SrcServer) == "" && strings.TrimSpace(req.SrcRole) != "" {
		roleCtx, err := tool.ResolveExecutionRole(ctx, "", req.SrcRole, "", "server_copy")
		if err != nil {
			return err
		}
		if strings.TrimSpace(roleCtx.Server) == "" {
			return fmt.Errorf("source role %s does not define a server", req.SrcRole)
		}
		req.SrcServer = roleCtx.Server
	}
	if strings.TrimSpace(req.DstServer) == "" && strings.TrimSpace(req.DstRole) != "" {
		roleCtx, err := tool.ResolveExecutionRole(ctx, "", req.DstRole, "", "server_copy")
		if err != nil {
			return err
		}
		if strings.TrimSpace(roleCtx.Server) == "" {
			return fmt.Errorf("destination role %s does not define a server", req.DstRole)
		}
		req.DstServer = roleCtx.Server
	}
	return nil
}

func requireExplicitCopyEndpoints(ctx tool.Context, req copyRequest) error {
	if !tool.RequiresExplicitServerAlias(ctx) {
		return nil
	}
	srcRaw := strings.TrimSpace(req.Src)
	if srcRaw == "" {
		srcRaw = strings.TrimSpace(req.SrcPath)
	}
	dstRaw := strings.TrimSpace(req.Dst)
	if dstRaw == "" {
		dstRaw = strings.TrimSpace(req.DstPath)
	}
	srcExplicit := hasExplicitEndpointAlias(ctx, srcRaw, req.SrcServer)
	dstExplicit := hasExplicitEndpointAlias(ctx, dstRaw, req.DstServer)
	if srcExplicit && dstExplicit {
		return nil
	}
	aliases := tool.RegisteredWorkspaceAliases(ctx)
	return fmt.Errorf("multiple remote workspaces are registered; server_copy requires explicit source and destination aliases. Use src/dst as alias:path or provide both src_server and dst_server. Available aliases: %s", strings.Join(aliases, ", "))
}

// lookupFileSize returns the size of a single file at the given endpoint.
// For local endpoints we use os.Stat directly. For remote endpoints we list
// the parent directory and match the basename, because ListDir on a single
// file path returns no entries.
func lookupFileSize(ctx tool.Context, ep workspace.EndpointPath) int64 {
	if !ep.IsRemote {
		if info, err := os.Stat(ep.RawPath); err == nil && !info.IsDir() {
			return info.Size()
		}
		return 0
	}
	if ctx.Workspace == nil {
		return 0
	}
	parent, base := splitRemotePath(ep)
	if parent == "" || base == "" {
		return 0
	}
	entries, err := ctx.Workspace.ListDir(ctx.Context, ep.Alias, parent, false, 0)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		if entry.Name == base {
			return entry.Size
		}
	}
	return 0
}

// splitRemotePath splits an endpoint path into (parent, base) using the
// separator appropriate for the endpoint OS.
func splitRemotePath(ep workspace.EndpointPath) (string, string) {
	raw := strings.TrimSpace(ep.RawPath)
	if raw == "" {
		return "", ""
	}
	if strings.EqualFold(ep.OSType, "windows") {
		dir := filepath.Dir(raw)
		base := filepath.Base(raw)
		return dir, base
	}
	return path.Dir(raw), path.Base(raw)
}

func hasExplicitEndpointAlias(ctx tool.Context, endpointRaw, serverField string) bool {
	if strings.TrimSpace(serverField) != "" {
		return true
	}
	token := strings.TrimSpace(endpointRaw)
	if token == "" {
		return false
	}

	isWindowsDrive := len(token) >= 3 &&
		token[1] == ':' &&
		(token[2] == '\\' || token[2] == '/') &&
		((token[0] >= 'a' && token[0] <= 'z') || (token[0] >= 'A' && token[0] <= 'Z'))
	if isWindowsDrive {
		return false
	}

	idx := strings.Index(token, ":")
	if idx <= 0 {
		return false
	}
	potentialAlias := strings.TrimSpace(token[:idx])
	if potentialAlias == "" {
		return false
	}
	if ctx.Workspace != nil && ctx.Workspace.IsKnownAlias(potentialAlias) {
		return true
	}
	return len(potentialAlias) > 1
}
