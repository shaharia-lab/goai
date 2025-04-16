package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StateStore defines the interface for storing and retrieving task state
type StateStore interface {
	SaveTaskState(taskID string, state *TaskState) error
	LoadTaskState(taskID string) (*TaskState, error)
	ListTaskIDs() ([]string, error)
}

// FileStateStore implements StateStore using the filesystem
type FileStateStore struct {
	BaseDir string
}

// NewFileStateStore creates a new file-based state store
func NewFileStateStore(baseDir string) (*FileStateStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}
	return &FileStateStore{BaseDir: baseDir}, nil
}

// SaveTaskState saves the task state to a file
func (f *FileStateStore) SaveTaskState(taskID string, state *TaskState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal task state: %w", err)
	}

	filePath := filepath.Join(f.BaseDir, taskID+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadTaskState loads the task state from a file
func (f *FileStateStore) LoadTaskState(taskID string) (*TaskState, error) {
	filePath := filepath.Join(f.BaseDir, taskID+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state TaskState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task state: %w", err)
	}

	return &state, nil
}

// ListTaskIDs returns a list of all task IDs in the store
func (f *FileStateStore) ListTaskIDs() ([]string, error) {
	files, err := os.ReadDir(f.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var taskIDs []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			taskIDs = append(taskIDs, file.Name()[:len(file.Name())-5])
		}
	}

	return taskIDs, nil
}
