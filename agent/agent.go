// agent/agent.go
package agent

import (
	"context"
	"fmt"
	"github.com/shaharia-lab/goai"
	"time"
)

// Agent represents the AI agent that executes tasks
type Agent struct {
	Runner     *TaskRunner
	StateStore StateStore
}

// NewAgent creates a new AI agent
func NewAgent(llm *goai.LLMRequest, stateDir string) (*Agent, error) {
	// Set up the state store
	stateStore, err := NewFileStateStore(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	// Set up the step executor
	executor := NewLLMStepExecutor(llm)
	taskStore, err := NewFileTaskStore("/home/shaharia/Projects/goai/data_store")

	// Set up the task runner
	taskRunner := NewTaskRunner(executor, stateStore, taskStore)

	return &Agent{
		Runner:     taskRunner,
		StateStore: stateStore,
	}, nil
}

// StartTask creates a new task and sets up its initial state
func (a *Agent) StartTask(ctx context.Context, t *Task) (string, error) {
	// Save the task to the task store
	if err := a.Runner.TaskStore.SaveTask(t); err != nil {
		return "", fmt.Errorf("failed to save task: %w", err)
	}

	// Initialize task state
	taskState := &TaskState{
		TaskID:         t.ID,
		StartTime:      time.Now(),
		CompletedSteps: make(map[string]StepResult),
		CurrentStepID:  "",
		IsComplete:     false,
	}

	// Save the initial state
	if err := a.StateStore.SaveTaskState(t.ID, taskState); err != nil {
		return "", fmt.Errorf("failed to save initial task state: %w", err)
	}

	return t.ID, nil
}

// RunTaskStep executes a single step of a task
func (a *Agent) RunTaskStep(ctx context.Context, taskID string) (bool, error) {
	return a.Runner.RunNextStep(ctx, taskID)
}

// RunTaskToCompletion runs all steps in a task until completion
// Use this if you want to run the entire task in one go
func (a *Agent) RunTaskToCompletion(ctx context.Context, taskID string) error {
	for {
		complete, err := a.RunTaskStep(ctx, taskID)
		if err != nil {
			return err
		}

		if complete {
			return nil
		}
	}
}

// GetTaskState returns the current state of a task
func (a *Agent) GetTaskState(taskID string) (*TaskState, error) {
	return a.StateStore.LoadTaskState(taskID)
}
