package task

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tasks"
	"github.com/koreaf16/argus/internal/tools"
)

type TaskCreateTool struct{}

func NewTaskCreateTool() *TaskCreateTool {
	return &TaskCreateTool{}
}

func (t *TaskCreateTool) Name() string {
	return "task_create"
}

func (t *TaskCreateTool) Description(ctx tools.Context) string {
	return "다단계 작업(설치, 마이그레이션, 빌드 파이프라인 등)을 추적하기 위한 새 작업 항목을 등록합니다. 작업이 시작되면 TaskUpdate로 in_progress, 완료 시 completed로 갱신하세요. 3단계 이상이거나 사용자에게 진행 상황을 보고할 가치가 있는 작업이라면 반드시 사용합니다."
}

func (t *TaskCreateTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"subject":     map[string]any{"type": "string", "description": "명령형 짧은 제목 (예: 'Oracle DB 19c 설치 파일 전송')"},
			"description": map[string]any{"type": "string", "description": "수행할 작업의 구체적 내용"},
			"active_form": map[string]any{"type": "string", "description": "in_progress 상태일 때 표시할 현재 진행형 문구 (예: 'Oracle DB 설치 파일 전송 중')"},
			"task_name":   map[string]any{"type": "string", "description": "(레거시) subject 미설정 시 fallback으로 사용"},
		},
	}
}

func (t *TaskCreateTool) IsReadOnly() bool {
	return false
}

func (t *TaskCreateTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 4)

	var req struct {
		Subject     string `json:"subject"`
		Description string `json:"description"`
		ActiveForm  string `json:"active_form"`
		TaskName    string `json:"task_name"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Subject)
	if name == "" {
		name = strings.TrimSpace(req.TaskName)
	}
	if name == "" {
		return nil, fmt.Errorf("subject 또는 task_name 중 하나는 반드시 지정해야 합니다")
	}

	go func() {
		defer close(out)
		task, err := tasks.SaveTaskFull(tasks.TaskInput{
			Name:        name,
			Description: strings.TrimSpace(req.Description),
			ActiveForm:  strings.TrimSpace(req.ActiveForm),
		})
		if err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		out <- tools.NewOutputEvent(fmt.Sprintf("작업 #%d '%s' 등록 완료 (id=%s)", task.Order, task.Name, task.ID))
		out <- tools.NewDoneEvent()
	}()

	return out, nil
}

func (t *TaskCreateTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}

func (t *TaskCreateTool) MaxResultSizeChars() int {
	return 1000
}
