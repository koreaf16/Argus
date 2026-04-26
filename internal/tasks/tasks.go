package tasks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/koreaf16/argus/internal/constants"
)

type Task struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

func SaveTask(name string) error {
	task := Task{
		ID:        time.Now().Format("20060102-150405"),
		Name:      name,
		CreatedAt: time.Now(),
		Status:    "created",
	}

	dir := filepath.Join(constants.ConfigDir(), "tasks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, task.ID+".json"), data, 0644)
}
