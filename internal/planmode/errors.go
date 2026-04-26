// Package planmode — Plan mode error types.
//
// 파일 역할: Defines error types for plan mode operations.
// 포함 모듈:
//   - ErrTaskNotFound: Task not found in plan
//
// 호출/사용 방식:
//   - plans.go and other plan functions
//
// 연결:
//   - internal/planmode
package planmode

import "errors"

// ErrTaskNotFound is returned when a task is not found in a plan.
var ErrTaskNotFound = errors.New("task not found in plan")

// ErrPlanNotFound is returned when a plan is not found.
var ErrPlanNotFound = errors.New("plan not found")

// ErrInvalidPlan is returned when a plan is invalid.
var ErrInvalidPlan = errors.New("invalid plan")
