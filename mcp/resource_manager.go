// resource_manager.go
package mcp

import (
	"fmt"
	"sync"
)

type Resource struct {
	URI         string                 `json:"uri"`
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ResourceManager struct {
	resources     map[string]*Resource
	subscriptions map[string][]*Connection
	mu            sync.RWMutex
}

func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		resources:     make(map[string]*Resource),
		subscriptions: make(map[string][]*Connection),
	}
}

func (rm *ResourceManager) AddResource(resource *Resource) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[resource.URI]; exists {
		return fmt.Errorf("resource %s already exists", resource.URI)
	}

	rm.resources[resource.URI] = resource
	rm.notifyResourceListChanged()
	return nil
}

func (rm *ResourceManager) GetResource(uri string) (*Resource, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resource, exists := rm.resources[uri]
	if !exists {
		return nil, fmt.Errorf("resource %s not found", uri)
	}

	return resource, nil
}

func (rm *ResourceManager) ListResources() []*Resource {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resources := make([]*Resource, 0, len(rm.resources))
	for _, r := range rm.resources {
		resources = append(resources, r)
	}
	return resources
}

func (rm *ResourceManager) Subscribe(uri string, conn *Connection) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.subscriptions[uri] = append(rm.subscriptions[uri], conn)
}

func (rm *ResourceManager) notifyResourceListChanged() {
	notification := Message{
		JSONRPC: "2.0",
		Method:  "notifications/resources/list_changed",
		Params:  nil,
	}

	for _, conns := range rm.subscriptions {
		for _, conn := range conns {
			_ = conn.SendMessage(notification)
		}
	}
}
