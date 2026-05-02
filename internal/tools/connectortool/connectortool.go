package connectortool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/connector"
	"github.com/koreaf16/argus/internal/tools"
)

type ConnectorSuggestTool struct {
	manager *connector.Manager
}

func New(manager *connector.Manager) *ConnectorSuggestTool {
	return &ConnectorSuggestTool{manager: manager}
}

func (t *ConnectorSuggestTool) Name() string {
	return "connector_suggest"
}

func (t *ConnectorSuggestTool) Description(ctx tools.Context) string {
	return "Search for and optionally install an MCP connector from the registry. Use this when you need a specific tool (e.g. database connector, kubernetes connector) that is currently missing."
}

func (t *ConnectorSuggestTool) IsVisible(ctx tools.Context) bool {
	return false
}

func (t *ConnectorSuggestTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Technology or tool keyword (e.g., 'oracle', 'mysql', 'k8s')",
			},
			"install": map[string]any{
				"type":        "string",
				"description": "Exact name of the connector to install if you already know it (optional). If provided, it will attempt to install it.",
			},
		},
		"required": []string{"query"},
	}
}

func (t *ConnectorSuggestTool) IsReadOnly() bool {
	return false
}

func (t *ConnectorSuggestTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAskPermission(), nil
}

func (t *ConnectorSuggestTool) MaxResultSizeChars() int {
	return 4000
}

func (t *ConnectorSuggestTool) Call(deps tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("connector manager is unavailable")
	}

	var req struct {
		Query   string `json:"query"`
		Install string `json:"install"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	out := make(chan tools.ToolEvent, 1)
	go func() {
		defer close(out)

		installName := strings.TrimSpace(req.Install)
		if installName != "" {
			spec, err := t.manager.Aggregator.Info(deps.Context, installName)
			if err != nil {
				out <- tools.NewErrorEvent(fmt.Errorf("failed to get connector info for %q: %v", installName, err))
				return
			}

			envAnswers := make(map[string]string)
			if len(spec.EnvPrompts) > 0 {
				var missing []string
				for _, ep := range spec.EnvPrompts {
					if ep.Required {
						missing = append(missing, ep.Key)
					}
				}
				if len(missing) > 0 {
					out <- tools.NewOutputEvent(fmt.Sprintf("Warning: Connector %q requires environment variables: %s.\nFor a better setup experience, please use the TUI installer.", installName, strings.Join(missing, ", ")))
				}
			}

			out <- tools.NewOutputEvent(fmt.Sprintf("Installing %s (runtime: %s)...", installName, spec.Runtime))
			if err := t.manager.Installer.Install(deps.Context, *spec, envAnswers); err != nil {
				out <- tools.NewErrorEvent(fmt.Errorf("failed to install %s: %v", installName, err))
				return
			}

			out <- tools.NewOutputEvent(fmt.Sprintf("Successfully installed connector %s. You may now use its tools.", installName))
			out <- tools.NewDoneEvent()
			return
		}

		query := strings.TrimSpace(req.Query)
		if query == "" {
			out <- tools.NewErrorEvent(fmt.Errorf("query is required"))
			return
		}

		results, err := t.manager.Aggregator.Search(deps.Context, query)
		if err != nil {
			out <- tools.NewErrorEvent(fmt.Errorf("search failed: %v", err))
			return
		}

		if len(results) == 0 {
			out <- tools.NewOutputEvent("No connectors found in the registry for this query.")
			out <- tools.NewDoneEvent()
			return
		}

		var sb strings.Builder
		sb.WriteString("Found the following connectors:\n")
		for i, r := range results {
			sb.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, r.Name, r.Description))
		}
		sb.WriteString("\nIf you want to use one, call this tool again with the 'install' parameter set to the exact name.")
		out <- tools.NewOutputEvent(sb.String())
		out <- tools.NewDoneEvent()
	}()
	return out, nil
}
