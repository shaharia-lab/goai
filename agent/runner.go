package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/shaharia-lab/goai"
	"log"
	"time"
)

// ErrTaskIncomplete indicates the task isn't finished yet
var ErrTaskIncomplete = errors.New("task execution is not complete")

// LLMStepExecutor executes steps using an LLM
type LLMStepExecutor struct {
	llm *goai.LLMRequest
}

// NewLLMStepExecutor creates a new LLM-based step executor
func NewLLMStepExecutor(llm *goai.LLMRequest) *LLMStepExecutor {
	return &LLMStepExecutor{
		llm: llm,
	}
}

// ExecuteStep runs a single step of a task
func (e *LLMStepExecutor) ExecuteStep(ctx context.Context, t *Task, step *Step, state *TaskState) (*StepResult, error) {
	// Build prompt from task context and step instructions
	prompt := buildPrompt(t, step, state)

	// Create context with timeout for this step
	stepCtx, cancel := context.WithTimeout(ctx, step.TimeoutDuration)
	defer cancel()

	// Execute the step
	startTime := time.Now()
	output, err := e.llm.Generate(stepCtx, []goai.LLMMessage{
		{
			Role: goai.UserRole,
			Text: prompt,
		},
	})
	endTime := time.Now()

	log.Printf("%s: %s", step.ID, output.Text)

	if err != nil {
		return &StepResult{
			StepID:    step.ID,
			Output:    "",
			StartTime: startTime,
			EndTime:   endTime,
			Success:   false,
			Error:     err.Error(),
		}, fmt.Errorf("step execution failed: %w", err)
	}

	// Return successful result
	return &StepResult{
		StepID:    step.ID,
		Output:    output.Text,
		StartTime: startTime,
		EndTime:   endTime,
		Success:   true,
	}, nil
}

// buildPrompt creates a prompt for the LLM based on the task, step, and current state
func buildPrompt(t *Task, step *Step, state *TaskState) string {
	prompt := fmt.Sprintf("Task: %s\n\nCurrent Step: %s\n\nInstructions: %s\n\n",
		t.Description, step.Description, step.Instructions)

	// Add context from previous steps if available
	if len(state.CompletedSteps) > 0 {
		prompt += "Previous steps results:\n\n"
		for _, prevStepID := range t.StepOrder {
			if result, exists := state.CompletedSteps[prevStepID]; exists {
				prompt += fmt.Sprintf("Step '%s' output:\n%s\n\n", prevStepID, result.Output)
			}
		}
	}

	prompt += "Please complete this step based on the instructions and context provided."
	return prompt
}

// TaskRunner manages the execution of a task
type TaskRunner struct {
	Executor   StepExecutor
	StateStore StateStore
	TaskStore  TaskStore
}

// NewTaskRunner creates a new task runner
func NewTaskRunner(executor StepExecutor, stateStore StateStore, taskStore TaskStore) *TaskRunner {
	return &TaskRunner{
		Executor:   executor,
		StateStore: stateStore,
		TaskStore:  taskStore,
	}
}

// RunNextStep executes the next step in a task
func (r *TaskRunner) RunNextStep(ctx context.Context, taskID string) (bool, error) {
	// Load the current t state
	taskState, err := r.StateStore.LoadTaskState(taskID)
	if err != nil {
		return false, fmt.Errorf("failed to load t state: %w", err)
	}

	// Load the t definition
	t, err := r.TaskStore.LoadTask(taskID) // Replace loadTask with this
	if err != nil {
		return false, fmt.Errorf("failed to load task: %w", err)
	}

	// Check if the t is already complete
	if taskState.IsComplete {
		return true, nil
	}

	// Determine the next step to execute
	nextStepID, err := determineNextStep(t, taskState)
	if err != nil {
		return false, err
	}

	if nextStepID == "" {
		// No more steps to execute, t is complete
		taskState.IsComplete = true
		taskState.FinalOutput = compileFinalOutput(t, taskState)
		err = r.StateStore.SaveTaskState(taskID, taskState)
		return true, err
	}

	// Execute the next step
	nextStep := t.Steps[nextStepID]
	taskState.CurrentStepID = nextStepID

	// Save state before executing step
	if err := r.StateStore.SaveTaskState(taskID, taskState); err != nil {
		return false, fmt.Errorf("failed to save pre-execution state: %w", err)
	}

	// Execute the step
	result, err := r.Executor.ExecuteStep(ctx, t, nextStep, taskState)
	if err != nil {
		// Update state to reflect failure
		result.Success = false
		result.Error = err.Error()
		taskState.CompletedSteps[nextStepID] = *result
		if saveErr := r.StateStore.SaveTaskState(taskID, taskState); saveErr != nil {
			return false, fmt.Errorf("step failed and couldn't save state: %v (original error: %w)", saveErr, err)
		}
		return false, fmt.Errorf("step execution failed: %w", err)
	}

	// Update state with successful result
	if taskState.CompletedSteps == nil {
		taskState.CompletedSteps = make(map[string]StepResult)
	}
	taskState.CompletedSteps[nextStepID] = *result

	// Save updated state
	if err := r.StateStore.SaveTaskState(taskID, taskState); err != nil {
		return false, fmt.Errorf("failed to save updated state: %w", err)
	}

	// There are more steps to execute
	return false, nil
}

// determineNextStep finds the next step to execute based on dependencies and completion status
func determineNextStep(task *Task, state *TaskState) (string, error) {
	// If we're using explicit step order
	if len(task.StepOrder) > 0 {
		for _, stepID := range task.StepOrder {
			if _, completed := state.CompletedSteps[stepID]; !completed {
				return stepID, nil
			}
		}
		return "", nil // All steps completed
	}

	// Otherwise, use dependencies to determine next steps
	for stepID, step := range task.Steps {
		if _, completed := state.CompletedSteps[stepID]; completed {
			continue // Skip completed steps
		}

		// Check if all dependencies are satisfied
		dependenciesMet := true
		for _, depID := range step.DependsOn {
			if _, depCompleted := state.CompletedSteps[depID]; !depCompleted {
				dependenciesMet = false
				break
			}
		}

		if dependenciesMet {
			return stepID, nil
		}
	}

	// Check if we have any incomplete steps with unsatisfiable dependencies
	for stepID, _ := range task.Steps {
		if _, completed := state.CompletedSteps[stepID]; !completed {
			return "", fmt.Errorf("step %s has unsatisfiable dependencies", stepID)
		}
	}

	return "", nil // All steps completed
}

// compileFinalOutput generates the final output based on all completed steps
func compileFinalOutput(task *Task, state *TaskState) string {
	// Simple implementation - combine outputs from all steps
	result := fmt.Sprintf("Task: %s\n\nFinal Output:\n\n", task.Name)

	for _, stepID := range task.StepOrder {
		if stepResult, exists := state.CompletedSteps[stepID]; exists {
			step := task.Steps[stepID]
			result += fmt.Sprintf("=== Step: %s ===\n%s\n\n", step.Description, stepResult.Output)
		}
	}

	return result
}

// loadTask is a placeholder - you'd implement this based on your task storage mechanism
func loadTask(taskID string) (*Task, error) {
	// This is just a placeholder - implement based on your storage
	return nil, fmt.Errorf("not implemented")
}
