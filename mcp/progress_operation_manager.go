// progress_manager.go
package mcp

import "fmt"

type ProgressManager struct {
	activeOperations map[string]*ProgressOperation
}

type ProgressOperation struct {
	ID       string
	Total    int64
	Current  int64
	Message  string
	Canceled bool
}

func NewProgressManager() *ProgressManager {
	return &ProgressManager{
		activeOperations: make(map[string]*ProgressOperation),
	}
}

func (pm *ProgressManager) CreateOperation(id string, total int64) *ProgressOperation {
	op := &ProgressOperation{
		ID:      id,
		Total:   total,
		Current: 0,
	}
	pm.activeOperations[id] = op
	return op
}

func (pm *ProgressManager) UpdateProgress(id string, current int64, message string) error {
	op, exists := pm.activeOperations[id]
	if !exists {
		return fmt.Errorf("operation %s not found", id)
	}

	op.Current = current
	op.Message = message

	// Send progress notification
	// TODO: Implement notification sending
	return nil
}
