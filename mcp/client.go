package mcp

import (
	"sync"
)

type Client interface {
	Connect() error
	Close() error
	ListTools() ([]Tool, error)
	GetState() ConnectionState
	IsInitialized() bool
	GetCapabilities() Capabilities
	GetProtocolVersion() string
}

type BaseClient struct {
	state            ConnectionState
	initialized      bool
	capabilities     Capabilities
	protocolVersion  string
	responseHandlers map[string]chan *Response
	retryAttempt     int
	stopChan         chan struct{}
	mu               sync.RWMutex
}

func NewBaseClient() *BaseClient {
	return &BaseClient{
		state:            Disconnected,
		responseHandlers: make(map[string]chan *Response),
		stopChan:         make(chan struct{}),
	}
}

func (c *BaseClient) GetState() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *BaseClient) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

func (c *BaseClient) GetCapabilities() Capabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capabilities
}

func (c *BaseClient) GetProtocolVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolVersion
}

func (c *BaseClient) setState(state ConnectionState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = state
}

func (c *BaseClient) setInitialized(initialized bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = initialized
}

func (c *BaseClient) setCapabilities(capabilities Capabilities) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capabilities = capabilities
}

func (c *BaseClient) setProtocolVersion(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.protocolVersion = version
}

func (c *BaseClient) addResponseHandler(id string, ch chan *Response) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responseHandlers[id] = ch
}

func (c *BaseClient) removeResponseHandler(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.responseHandlers, id)
}

func (c *BaseClient) getResponseHandler(id string) (chan *Response, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ch, exists := c.responseHandlers[id]
	return ch, exists
}

func (c *BaseClient) incrementRetryAttempt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.retryAttempt++
}

func (c *BaseClient) getRetryAttempt() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.retryAttempt
}

func (c *BaseClient) resetRetryAttempt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.retryAttempt = 0
}
