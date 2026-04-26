package skilltool

import (
	"encoding/json"
	"fmt"

	"github.com/koreaf16/argus/internal/skills"
	tool "github.com/koreaf16/argus/internal/tools"
)

type SkillTool struct {
	registry *skills.Registry
}

func NewSkillTool(r *skills.Registry) *SkillTool {
	return &SkillTool{registry: r}
}

func (t *SkillTool) Name() string { return "skill" }

func (t *SkillTool) Description(ctx tool.Context) string { return "Run a registered skill" }

func (t *SkillTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "The name of the skill to run"},
			"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Arguments to pass to the skill"},
		},
		"required": []string{"name"},
	}
}

func (t *SkillTool) IsReadOnly() bool { return false }

func (t *SkillTool) MaxResultSizeChars() int {
	return 100000
}

func (t *SkillTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func (t *SkillTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req struct {
		Name string   `json:"name"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ch := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(ch)

		if t.registry == nil {
			ch <- tool.NewErrorEvent(fmt.Errorf("skill registry is unavailable"))
			return
		}

		result, err := t.registry.Run(req.Name, req.Args)
		if err != nil {
			ch <- tool.NewErrorEvent(err)
			return
		}

		ch <- tool.NewOutputEvent(result)
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}
