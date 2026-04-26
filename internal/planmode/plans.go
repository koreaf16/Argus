// Package planmode — Plan storage and management.
//
// 파일 역할: Handles plan reading, writing, and execution in plan mode.
// 포함 모듈:
//   - Plan: Plan structure
//   - ReadPlan: Loads a plan from storage
//   - WritePlan: Saves a plan to storage
//   - ExecutePlan: Executes a plan
//
// 호출/사용 방식:
//   - Plan execution engines call these functions
//
// 연결:
//   - internal/planmode: planmode.go
//   - 원본: src/utils/planModeV2.ts, plans.ts
package planmode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Plan represents a structured task plan.
type Plan struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tasks       []Task    `json:"tasks"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Status      string    `json:"status"` // "pending", "in_progress", "completed"
}

// Task represents a single task in a plan.
type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"` // "pending", "in_progress", "completed"
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// ReadPlan reads a plan from disk.
func ReadPlan(planPath string) (*Plan, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, err
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}

	return &plan, nil
}

// WritePlan writes a plan to disk.
func WritePlan(plan *Plan, planPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(planPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Update timestamp
	plan.UpdatedAt = time.Now()

	// Marshal and write
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(planPath, data, 0600)
}

// ExecutePlan executes a plan by running its tasks.
// Phase 1: This is a stub; real execution is Phase 2.
func ExecutePlan(plan *Plan) error {
	// Phase 1: Stub implementation
	// Real implementation would:
	// 1. Create tasks
	// 2. Execute each task
	// 3. Track progress
	// 4. Handle failures
	// 5. Report results

	return nil
}

// UpdateTaskStatus updates the status of a task in a plan.
func UpdateTaskStatus(plan *Plan, taskID, newStatus string) error {
	for i, task := range plan.Tasks {
		if task.ID == taskID {
			plan.Tasks[i].Status = newStatus

			now := time.Now()
			if newStatus == "in_progress" {
				plan.Tasks[i].StartedAt = &now
			} else if newStatus == "completed" {
				plan.Tasks[i].CompletedAt = &now
			}

			return nil
		}
	}

	return ErrTaskNotFound
}

// GetPlanStatus returns the overall status of a plan based on its tasks.
func GetPlanStatus(plan *Plan) string {
	if len(plan.Tasks) == 0 {
		return "pending"
	}

	completedCount := 0
	for _, task := range plan.Tasks {
		if task.Status == "completed" {
			completedCount++
		}
	}

	if completedCount == len(plan.Tasks) {
		return "completed"
	} else if completedCount > 0 {
		return "in_progress"
	}

	return "pending"
}
