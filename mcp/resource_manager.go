// resource_manager.go
package mcp

import (
	"fmt"
	"sync"
)

type ResourceManager struct {
	mu          sync.RWMutex
	resources   map[string]Resource
	templates   map[string]ResourceTemplate
	subscribers map[string][]chan ResourceChange
}

type Resource struct {
	URI          string                 `json:"uri"`
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	Properties   map[string]interface{} `json:"properties"`
	LastModified int64                  `json:"lastModified"`
}

type ResourceTemplate struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Properties  []PropertyDefinition `json:"properties"`
}

type PropertyDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type ResourceChange struct {
	Type     string   `json:"type"` // "created", "updated", "deleted"
	Resource Resource `json:"resource"`
}

func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		resources:   make(map[string]Resource),
		templates:   make(map[string]ResourceTemplate),
		subscribers: make(map[string][]chan ResourceChange),
	}
}

func (rm *ResourceManager) RegisterTemplate(template ResourceTemplate) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.templates[template.ID]; exists {
		return fmt.Errorf("template %s already exists", template.ID)
	}

	rm.templates[template.ID] = template
	return nil
}

func (rm *ResourceManager) CreateResource(templateID string, properties map[string]interface{}) (*Resource, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	template, exists := rm.templates[templateID]
	if !exists {
		return nil, fmt.Errorf("template %s not found", templateID)
	}

	// Validate properties against template
	if err := rm.validateProperties(template, properties); err != nil {
		return nil, err
	}

	resource := Resource{
		URI:          generateResourceURI(),
		Type:         template.ID,
		Name:         properties["name"].(string),
		Properties:   properties,
		LastModified: getCurrentTimestamp(),
	}

	rm.resources[resource.URI] = resource

	// Notify subscribers
	rm.notifyChange(ResourceChange{
		Type:     "created",
		Resource: resource,
	})

	return &resource, nil
}

func (rm *ResourceManager) GetResource(uri string) (*Resource, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resource, exists := rm.resources[uri]
	if !exists {
		return nil, fmt.Errorf("resource %s not found", uri)
	}

	return &resource, nil
}

func (rm *ResourceManager) ListResources() []Resource {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resources := make([]Resource, 0, len(rm.resources))
	for _, resource := range rm.resources {
		resources = append(resources, resource)
	}

	return resources
}

func (rm *ResourceManager) ListTemplates() []ResourceTemplate {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	templates := make([]ResourceTemplate, 0, len(rm.templates))
	for _, template := range rm.templates {
		templates = append(templates, template)
	}

	return templates
}

func (rm *ResourceManager) Subscribe(subscriberID string) chan ResourceChange {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	ch := make(chan ResourceChange, 100)
	rm.subscribers[subscriberID] = append(rm.subscribers[subscriberID], ch)
	return ch
}

func (rm *ResourceManager) Unsubscribe(subscriberID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if channels, exists := rm.subscribers[subscriberID]; exists {
		for _, ch := range channels {
			close(ch)
		}
		delete(rm.subscribers, subscriberID)
	}
}

func (rm *ResourceManager) notifyChange(change ResourceChange) {
	for _, channels := range rm.subscribers {
		for _, ch := range channels {
			select {
			case ch <- change:
			default:
				// Channel is full, skip notification
			}
		}
	}
}

func (rm *ResourceManager) validateProperties(template ResourceTemplate, properties map[string]interface{}) error {
	for _, prop := range template.Properties {
		if prop.Required {
			if _, exists := properties[prop.Name]; !exists {
				return fmt.Errorf("required property %s is missing", prop.Name)
			}
		}
	}
	return nil
}
