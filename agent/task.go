package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TaskStore defines the interface for task persistence operations
type TaskStore interface {
	SaveTask(task *Task) error
	LoadTask(taskID string) (*Task, error)
	ListTasks() ([]*Task, error)
	DeleteTask(taskID string) error
}

// File-based task storage using JSON
type FileTaskStore struct {
	dirPath string
}

func NewFileTaskStore(dirPath string) (*FileTaskStore, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task storage directory: %w", err)
	}
	return &FileTaskStore{dirPath: dirPath}, nil
}

func (s *FileTaskStore) taskPath(taskID string) string {
	return filepath.Join(s.dirPath, taskID+".json")
}

func (s *FileTaskStore) SaveTask(task *Task) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	err = os.WriteFile(s.taskPath(task.ID), data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write task file: %w", err)
	}

	return nil
}

func (s *FileTaskStore) LoadTask(taskID string) (*Task, error) {
	data, err := os.ReadFile(s.taskPath(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("task with ID %s not found", taskID)
		}
		return nil, fmt.Errorf("failed to read task file: %w", err)
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task data: %w", err)
	}

	return &task, nil
}

// List all tasks in the storage directory
func (s *FileTaskStore) ListTasks() ([]*Task, error) {
	files, err := os.ReadDir(s.dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task directory: %w", err)
	}

	var tasks []*Task
	for _, file := range files {
		// Skip directories and non-JSON files
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// Extract task ID from filename
		taskID := strings.TrimSuffix(file.Name(), ".json")

		// Load the task
		task, err := s.LoadTask(taskID)
		if err != nil {
			// You could choose to skip problematic files instead
			return nil, fmt.Errorf("failed to load task %s: %w", taskID, err)
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// Delete a task from storage
func (s *FileTaskStore) DeleteTask(taskID string) error {
	path := s.taskPath(taskID)

	// Check if the file exists first
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("task with ID %s not found", taskID)
		}
		return fmt.Errorf("failed to access task file: %w", err)
	}

	// Delete the file
	err = os.Remove(path)
	if err != nil {
		return fmt.Errorf("failed to delete task file: %w", err)
	}

	return nil
}
