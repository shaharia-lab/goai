package agent

import (
	"context"
	"time"
)

// Step represents a single unit of work in a task
type Step struct {
	ID              string
	Description     string
	Instructions    string
	TimeoutDuration time.Duration
	DependsOn       []string // IDs of steps this step depends on
}

// Task represents a complete job consisting of multiple steps
type Task struct {
	ID          string
	Name        string
	Description string
	Steps       map[string]*Step
	StepOrder   []string // Optional - defines execution order if not using dependencies
}

// StepResult contains the output of a completed step
type StepResult struct {
	StepID    string
	Output    string
	StartTime time.Time
	EndTime   time.Time
	Success   bool
	Error     string
}

// TaskState tracks the current state of a task execution
type TaskState struct {
	TaskID         string
	StartTime      time.Time
	CompletedSteps map[string]StepResult // Note: using StepResult directly, not a pointer
	CurrentStepID  string
	IsComplete     bool
	FinalOutput    string
}

// StepExecutor defines the interface for executing a step
type StepExecutor interface {
	ExecuteStep(ctx context.Context, task *Task, step *Step, state *TaskState) (*StepResult, error)
}
