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
)

type Task struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

func tasksDir() string {
	return filepath.Join(constants.ConfigDir(), "tasks")
}

func SaveTask(name string) error {
	task := Task{
		ID:        time.Now().Format("20060102-150405"),
		Name:      name,
		CreatedAt: time.Now(),
		Status:    "created",
	}

	if err := os.MkdirAll(tasksDir(), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(tasksDir(), task.ID+".json"), data, 0644)
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

func UpdateTaskStatus(id, status string) error {
	t, err := GetTask(id)
	if err != nil {
		return err
	}
	t.Status = status

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tasksDir(), id+".json"), data, 0644)
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
