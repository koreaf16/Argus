package servercopy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerCopyTool struct{}

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
			"src_path":   map[string]any{"type": "string", "description": "Source file path"},
			"dst_server": map[string]any{"type": "string", "description": "Destination server alias"},
			"dst_path":   map[string]any{"type": "string", "description": "Destination file path"},
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

	var req struct {
		Src       string `json:"src"`
		Dst       string `json:"dst"`
		SrcServer string `json:"src_server"`
		SrcPath   string `json:"src_path"`
		DstServer string `json:"dst_server"`
		DstPath   string `json:"dst_path"`
		Overwrite *bool  `json:"overwrite"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		overwrite := true
		if req.Overwrite != nil {
			overwrite = *req.Overwrite
		}
		srcAlias, srcPath, dstAlias, dstPath, err := parseCopyRequest(req.Src, req.SrcServer, req.SrcPath, req.Dst, req.DstServer, req.DstPath, ctx.Workspace.ActiveAlias())
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if err := ctx.Workspace.CopyFile(ctx.Context, srcAlias, srcPath, dstAlias, dstPath, overwrite); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		events <- tool.NewOutputEvent(fmt.Sprintf("copied %s:%s -> %s:%s", srcAlias, srcPath, dstAlias, dstPath))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerCopyTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return types.PermissionResult{Behavior: types.BehaviorAllow}, nil
}

func parseCopyRequest(srcEndpoint, srcServer, srcPath, dstEndpoint, dstServer, dstPath, activeAlias string) (string, string, string, string, error) {
	srcAlias := strings.TrimSpace(srcServer)
	dstAlias := strings.TrimSpace(dstServer)
	src := strings.TrimSpace(srcPath)
	dst := strings.TrimSpace(dstPath)

	// src/dst endpoint 파싱 (req.Src / req.Dst 필드)
	if strings.TrimSpace(srcEndpoint) != "" {
		parsedAlias, parsedPath, parseErr := workspace.ParseEndpointPath(srcEndpoint, activeAlias)
		if parseErr == nil {
			if srcAlias == "" {
				srcAlias = parsedAlias
			}
			if src == "" {
				src = parsedPath
			}
		}
	}
	if strings.TrimSpace(dstEndpoint) != "" {
		parsedAlias, parsedPath, parseErr := workspace.ParseEndpointPath(dstEndpoint, activeAlias)
		if parseErr == nil {
			if dstAlias == "" {
				dstAlias = parsedAlias
			}
			if dst == "" {
				dst = parsedPath
			}
		}
	}

	// src_path / dst_path 에도 "alias:path" 형태가 있으면 파싱
	if src != "" {
		if parsedAlias, parsedPath, parseErr := workspace.ParseEndpointPath(src, ""); parseErr == nil && parsedPath != src {
			if srcAlias == "" {
				srcAlias = parsedAlias
			}
			src = parsedPath
		}
	}
	if dst != "" {
		if parsedAlias, parsedPath, parseErr := workspace.ParseEndpointPath(dst, ""); parseErr == nil && parsedPath != dst {
			if dstAlias == "" {
				dstAlias = parsedAlias
			}
			dst = parsedPath
		}
	}

	// 여전히 alias가 없는 경우 기본값 적용
	if srcAlias == "" && src != "" {
		srcAlias = workspace.LocalAlias
	}
	if dstAlias == "" && dst != "" {
		dstAlias = activeAlias
	}

	if srcAlias == "" || src == "" || dstAlias == "" || dst == "" {
		return "", "", "", "", fmt.Errorf("source and destination are required (src_server/src_path and dst_server/dst_path)")
	}
	return srcAlias, src, dstAlias, dst, nil
}
