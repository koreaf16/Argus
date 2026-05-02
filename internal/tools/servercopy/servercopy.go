package servercopy

import (
	"encoding/json"
	"fmt"
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
	return "Copy a file across registered workspaces (local/ssh). Use this when you need to move files between local PC and remote servers."
}

func (t *ServerCopyTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"src":        map[string]any{"type": "string", "description": "Source endpoint path (alias:path). Example: local:C:\\\\Users\\\\me\\\\Downloads\\\\a.zip"},
			"dst":        map[string]any{"type": "string", "description": "Destination endpoint path (alias:path). Example: dev:/tmp/a.zip"},
			"src_server": map[string]any{"type": "string", "description": "Source server alias (e.g. local, dev, prod)"},
			"src_role":   map[string]any{"type": "string", "description": "Source workflow role."},
			"src_path":   map[string]any{"type": "string", "description": "Source file path"},
			"dst_server": map[string]any{"type": "string", "description": "Destination server alias"},
			"dst_role":   map[string]any{"type": "string", "description": "Destination workflow role."},
			"dst_path":   map[string]any{"type": "string", "description": "Destination file path"},
			"role":       map[string]any{"type": "string", "description": "Optional transfer workflow role."},
			"channel":    map[string]any{"type": "string", "description": "Optional workflow channel."},
			"overwrite":  map[string]any{"type": "boolean", "description": "Whether to overwrite destination file"},
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
	events := make(chan tool.ToolEvent, 2)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("workspace manager is unavailable")
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
			events <- tool.NewErrorEvent(fmt.Errorf("source and destination are required (src_server/src_path and dst_server/dst_path)"))
			return
		}

		events <- tool.NewOutputEvent(fmt.Sprintf("Copying %s to %s...", srcEP.RawPath, dstEP.RawPath))

		// Get source file size for progress reporting
		var totalBytes int64
		if entries, err := ctx.Workspace.ListDir(ctx.Context, srcEP.Alias, srcEP.RawPath, false, 0); err == nil && len(entries) > 0 {
			totalBytes = entries[0].Size
		}

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
		events <- tool.NewOutputEvent(fmt.Sprintf("Successfully copied %s to %s", srcEP.RawPath, dstEP.RawPath))
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
	return fmt.Errorf("explicit source/destination server aliases are required for server_copy when multiple remote workspaces are registered. Use `src`/`dst` as alias:path or set both `src_server` and `dst_server`. Available aliases: %s", strings.Join(aliases, ", "))
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
