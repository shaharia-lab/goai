package mcp

import (
	"fmt"
	"sync"
)

// Resource represents a managed resource in the MCP system
type Resource struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Properties  map[string]interface{} `json:"properties"`
	Subscribers map[string]*Connection `json:"-"`
}

// ResourceManager handles resource lifecycle and subscriptions
type ResourceManager struct {
	resources map[string]*Resource
	mu        sync.RWMutex
}

// NewResourceManager creates a new resource manager instance
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		resources: make(map[string]*Resource),
	}
}

// AddResource registers a new resource
func (rm *ResourceManager) AddResource(resource *Resource) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[resource.ID]; exists {
		return fmt.Errorf("resource with ID %s already exists", resource.ID)
	}

	resource.Subscribers = make(map[string]*Connection)
	rm.resources[resource.ID] = resource
	return nil
}

// RemoveResource removes a resource and notifies subscribers
func (rm *ResourceManager) RemoveResource(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	resource, exists := rm.resources[id]
	if !exists {
		return fmt.Errorf("resource with ID %s not found", id)
	}

	// Notify subscribers about removal
	for _, conn := range resource.Subscribers {
		notification, _ := NewRequest(nil, "resource/removed", map[string]string{
			"id": id,
		})
		_ = conn.SendMessage(*notification)
	}

	delete(rm.resources, id)
	return nil
}

// Subscribe adds a connection as a subscriber to a resource
func (rm *ResourceManager) Subscribe(resourceID string, conn *Connection) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	resource, exists := rm.resources[resourceID]
	if !exists {
		return fmt.Errorf("resource with ID %s not found", resourceID)
	}

	resource.Subscribers[conn.ID] = conn
	return nil
}

// Unsubscribe removes a connection from a resource's subscribers
func (rm *ResourceManager) Unsubscribe(resourceID string, conn *Connection) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	resource, exists := rm.resources[resourceID]
	if !exists {
		return fmt.Errorf("resource with ID %s not found", resourceID)
	}

	delete(resource.Subscribers, conn.ID)
	return nil
}

// NotifyResourceChanged notifies all subscribers about resource changes
func (rm *ResourceManager) NotifyResourceChanged(resourceID string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resource, exists := rm.resources[resourceID]
	if !exists {
		return fmt.Errorf("resource with ID %s not found", resourceID)
	}

	notification, _ := NewRequest(nil, "resource/changed", map[string]interface{}{
		"id":         resourceID,
		"type":       resource.Type,
		"properties": resource.Properties,
	})

	for _, conn := range resource.Subscribers {
		_ = conn.SendMessage(*notification)
	}

	return nil
}
