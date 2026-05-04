package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/types"
)

const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

type Task struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ActiveForm  string    `json:"active_form,omitempty"`
	Order       int       `json:"order,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Status      string    `json:"status"`
}

type TaskInput struct {
	Name        string
	Description string
	ActiveForm  string
}

func tasksDir() string {
	return filepath.Join(constants.ConfigDir(), "tasks")
}

func nextOrder() int {
	list, err := ListTasks()
	if err != nil || len(list) == 0 {
		return 1
	}
	maxOrder := 0
	for _, t := range list {
		if t.Order > maxOrder {
			maxOrder = t.Order
		}
	}
	return maxOrder + 1
}

// SaveTask preserves the original signature for backward compatibility.
// New callers should prefer SaveTaskFull.
func SaveTask(name string) error {
	_, err := SaveTaskFull(TaskInput{Name: name})
	return err
}

// SaveTaskFull persists a task with rich metadata and returns it.
func SaveTaskFull(in TaskInput) (*Task, error) {
	now := time.Now()
	task := &Task{
		ID:          now.Format("20060102-150405.000"),
		Name:        in.Name,
		Description: in.Description,
		ActiveForm:  in.ActiveForm,
		Order:       nextOrder(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      StatusPending,
	}
	if err := writeTask(task); err != nil {
		return nil, err
	}
	return task, nil
}

func ListTasks() ([]Task, error) {
	entries, err := os.ReadDir(tasksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tasks []Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tasksDir(), e.Name()))
		if err != nil {
			continue
		}
		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Order != tasks[j].Order {
			return tasks[i].Order < tasks[j].Order
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks, nil
}

func GetTask(id string) (*Task, error) {
	data, err := os.ReadFile(filepath.Join(tasksDir(), id+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("task %q not found", id)
		}
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTaskStatus preserves the original signature.
func UpdateTaskStatus(id, status string) error {
	return UpdateTaskFields(id, TaskUpdate{Status: &status})
}

type TaskUpdate struct {
	Name        *string
	Description *string
	ActiveForm  *string
	Status      *string
	Order       *int
}

// UpdateTaskFields applies a partial update to an existing task.
func UpdateTaskFields(id string, upd TaskUpdate) error {
	t, err := GetTask(id)
	if err != nil {
		return err
	}
	if upd.Name != nil {
		t.Name = *upd.Name
	}
	if upd.Description != nil {
		t.Description = *upd.Description
	}
	if upd.ActiveForm != nil {
		t.ActiveForm = *upd.ActiveForm
	}
	if upd.Status != nil {
		t.Status = *upd.Status
	}
	if upd.Order != nil {
		t.Order = *upd.Order
	}
	t.UpdatedAt = time.Now()
	return writeTask(t)
}

func DeleteTask(id string) error {
	path := filepath.Join(tasksDir(), id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("task %q not found", id)
		}
		return err
	}
	return nil
}

// ClearAllTasks removes all tasks in the tasks directory.
func ClearAllTasks() error {
	return os.RemoveAll(tasksDir())
}

// AsTodoItems converts the current persisted tasks into a TodoItem snapshot
// suitable for the bottom-anchored TUI panel.
func AsTodoItems() []types.TodoItem {
	list, err := ListTasks()
	if err != nil || len(list) == 0 {
		return nil
	}
	out := make([]types.TodoItem, 0, len(list))
	for _, t := range list {
		out = append(out, types.TodoItem{
			Content:    t.Name,
			Status:     statusToTodoStatus(t.Status),
			ActiveForm: t.ActiveForm,
		})
	}
	return out
}

func statusToTodoStatus(s string) types.TodoStatus {
	switch s {
	case StatusCompleted:
		return types.TodoStatusCompleted
	case StatusInProgress:
		return types.TodoStatusInProgress
	default:
		return types.TodoStatusPending
	}
}

func writeTask(t *Task) error {
	if err := os.MkdirAll(tasksDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tasksDir(), t.ID+".json"), data, 0o644)
}
