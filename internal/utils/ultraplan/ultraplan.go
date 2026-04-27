// Package ultraplan — Plan execution coordination (Phase 2).
//
// CCRSession links a set of persistent tasks (internal/tasks) to a single
// coordinated execution session. The session is saved to disk so progress
// survives across restarts.
//
// Storage path: ~/.argus/ultraplan/{sessionID}.json
package ultraplan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/tasks"
)

// CCRSession coordinates the execution of a set of persistent tasks.
type CCRSession struct {
	ID        string              `json:"id"`
	StartedAt time.Time           `json:"started_at"`
	TaskIDs   []string            `json:"task_ids"`
}

// NewCCRSession creates a new coordination session with the given task IDs.
func NewCCRSession(sessionID string, taskIDs []string) *CCRSession {
	return &CCRSession{
		ID:        sessionID,
		StartedAt: time.Now(),
		TaskIDs:   taskIDs,
	}
}

// Save persists the session to disk.
func (s *CCRSession) Save() error {
	dir := filepath.Join(constants.ConfigDir(), "ultraplan")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.ID+".json"), data, 0644)
}

// LoadSession loads a previously saved CCRSession from disk.
func LoadSession(sessionID string) (*CCRSession, error) {
	path := filepath.Join(constants.ConfigDir(), "ultraplan", sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s CCRSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Tasks resolves and returns the actual task objects for this session.
func (s *CCRSession) Tasks() ([]*tasks.Task, error) {
	result := make([]*tasks.Task, 0, len(s.TaskIDs))
	for _, id := range s.TaskIDs {
		t, err := tasks.GetTask(id)
		if err != nil {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

// AdvanceTask updates the status of one task and saves the session.
func (s *CCRSession) AdvanceTask(taskID, status string) error {
	if err := tasks.UpdateTaskStatus(taskID, status); err != nil {
		return err
	}
	return s.Save()
}

// IsComplete returns true when all tasks in the session are "completed".
func (s *CCRSession) IsComplete() (bool, error) {
	taskList, err := s.Tasks()
	if err != nil {
		return false, err
	}
	for _, t := range taskList {
		if t.Status != "completed" {
			return false, nil
		}
	}
	return len(taskList) > 0, nil
}

// ElapsedTime returns how long the session has been running.
func (s *CCRSession) ElapsedTime() time.Duration {
	return time.Since(s.StartedAt)
}
