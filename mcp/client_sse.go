package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseRetryDelay = 5 * time.Second
	defaultMaxRetryDelay  = 30 * time.Second
	defaultMaxRetries     = 5
	defaultMaxMissedPings = 2
	defaultClientName     = "mcp-client"
	defaultClientVersion  = "1.0.0"
)

type ConnectionState int

const (
	Disconnected ConnectionState = iota
	Connecting
	Connected
)

type SSEClientConfig struct {
	URL           string
	RetryDelay    time.Duration
	MaxRetries    int
	ClientName    string
	ClientVersion string
	Logger        *log.Logger
}

type SSEClient struct {
	config           SSEClientConfig
	messageEndpoint  string
	clientID         string
	lastPingTime     time.Time
	missedPings      int
	retryAttempt     int
	state            ConnectionState
	stopChan         chan struct{}
	initialized      bool
	capabilities     Capabilities
	protocolVersion  string
	mu               sync.RWMutex
	responseHandlers map[string]chan *Response
	nextRequestID    int
	eventStream      *http.Response
	streamReader     *bufio.Reader
}

func NewSSEClient(config SSEClientConfig) *SSEClient {
	if config.RetryDelay == 0 {
		config.RetryDelay = defaultBaseRetryDelay
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.ClientName == "" {
		config.ClientName = defaultClientName
	}
	if config.ClientVersion == "" {
		config.ClientVersion = defaultClientVersion
	}
	if config.Logger == nil {
		config.Logger = log.Default()
	}

	return &SSEClient{
		config:           config,
		stopChan:         make(chan struct{}),
		state:            Disconnected,
		responseHandlers: make(map[string]chan *Response),
		nextRequestID:    1,
	}
}

func (c *SSEClient) Connect() error {
	c.mu.Lock()
	if c.state != Disconnected {
		c.mu.Unlock()
		return fmt.Errorf("client is already connected or connecting")
	}
	c.state = Connecting
	c.mu.Unlock()

	c.config.Logger.Println("Starting connection process...")

	// Step 1: Connect to SSE endpoint
	c.config.Logger.Printf("Connecting to SSE endpoint: %s", c.config.URL)
	resp, err := http.Get(c.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to events endpoint: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status code from events endpoint: %d", resp.StatusCode)
	}

	c.eventStream = resp
	c.streamReader = bufio.NewReader(resp.Body)

	// Start processing SSE stream
	go c.processEventStream()

	// Wait for message endpoint
	c.config.Logger.Println("Waiting for message endpoint...")
	if err := c.waitForMessageEndpoint(); err != nil {
		return err
	}
	c.config.Logger.Println("Message endpoint received")

	// Step 2: Send initialize request
	c.config.Logger.Println("Sending initialize request...")
	if err := c.sendInitializeRequest(); err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}
	c.config.Logger.Println("Initialize request successful")

	// Step 3: Send initialized notification
	c.config.Logger.Println("Sending initialized notification...")
	notification := Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := c.sendMessage(&notification); err != nil {
		return fmt.Errorf("failed to send initialized notification: %v", err)
	}
	c.config.Logger.Println("Initialized notification sent successfully")

	c.mu.Lock()
	c.state = Connected
	c.initialized = true
	c.mu.Unlock()

	go c.monitorPings()

	c.config.Logger.Println("Connection established successfully!")
	return nil
}

func (c *SSEClient) processEventStream() {
	scanner := bufio.NewScanner(c.eventStream.Body)
	for {
		select {
		case <-c.stopChan:
			return
		default:
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					c.config.Logger.Printf("SSE stream error: %v", err)
				}
				return
			}

			line := scanner.Text()
			c.config.Logger.Printf("SSE Raw line: %s", line)

			if line == "" {
				continue
			}

			if line == ":ping" {
				c.handlePing()
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") {
					c.handleMessageEndpoint(data)
				} else {
					c.handleResponse(data)
				}
			}
		}
	}
}

func (c *SSEClient) waitForMessageEndpoint() error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for message endpoint")
		case <-ticker.C:
			c.mu.RLock()
			if c.messageEndpoint != "" {
				c.mu.RUnlock()
				return nil
			}
			c.mu.RUnlock()
		case <-c.stopChan:
			return fmt.Errorf("client stopped")
		}
	}
}

func (c *SSEClient) handleMessageEndpoint(endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageEndpoint = endpoint
	c.clientID = c.extractClientID(endpoint)
	c.config.Logger.Printf("Received message endpoint: %s with client ID: %s", endpoint, c.clientID)
}

func (c *SSEClient) handleResponse(data string) {
	c.config.Logger.Printf("Handling response data: %s", data)

	var response Response
	if err := json.Unmarshal([]byte(data), &response); err != nil {
		c.config.Logger.Printf("Failed to parse response: %v", err)
		return
	}

	if response.ID != nil {
		idStr := strings.Trim(string(*response.ID), "\"")
		c.config.Logger.Printf("Processing response for request ID: %s", idStr)

		c.mu.RLock()
		ch, exists := c.responseHandlers[idStr]
		c.mu.RUnlock()

		if exists {
			c.config.Logger.Printf("Found handler for request ID: %s", idStr)
			select {
			case ch <- &response:
				c.config.Logger.Printf("Response sent to handler for request ID: %s", idStr)
			default:
				c.config.Logger.Printf("Handler channel full for request ID: %s", idStr)
			}
		} else {
			c.config.Logger.Printf("No handler found for request ID: %s", idStr)
		}
	}
}

func (c *SSEClient) sendInitializeRequest() error {
	responseChan := make(chan *Response, 1)
	requestID := "init"
	rawID := json.RawMessage(`"init"`)

	// Register response handler before sending request
	c.mu.Lock()
	c.responseHandlers[requestID] = responseChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.responseHandlers, requestID)
		c.mu.Unlock()
	}()

	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]any{
			"tools": map[string]bool{
				"listChanged": true,
			},
		},
		ClientInfo: struct {
			Name    string "json:\"name\""
			Version string "json:\"version\""
		}{
			Name:    c.config.ClientName,
			Version: c.config.ClientVersion,
		},
	}

	request := Request{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      &rawID,
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal initialize params: %v", err)
	}
	request.Params = paramsBytes

	c.config.Logger.Printf("Registered handler for request ID: %s", requestID)
	if err := c.sendMessage(&request); err != nil {
		return err
	}

	c.config.Logger.Println("Waiting for initialize response...")
	select {
	case response := <-responseChan:
		c.config.Logger.Println("Received initialize response")
		if response.Error != nil {
			return fmt.Errorf("server error: %s (code: %d)",
				response.Error.Message, response.Error.Code)
		}

		c.config.Logger.Printf("Response Result: %+v", response.Result)

		var result InitializeResult
		resultBytes, err := json.Marshal(response.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal response result: %v", err)
		}

		if err := json.Unmarshal(resultBytes, &result); err != nil {
			return fmt.Errorf("failed to parse initialize result: %v", err)
		}

		c.mu.Lock()
		c.capabilities = result.Capabilities
		c.protocolVersion = result.ProtocolVersion
		c.mu.Unlock()

		c.config.Logger.Printf("Server capabilities received: %+v", result.Capabilities)
		return nil

	case <-time.After(30 * time.Second):
		return fmt.Errorf("initialize request timeout")
	}
}

func (c *SSEClient) sendInitializedNotification() error {
	notification := Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.sendMessage(&notification)
}

func (c *SSEClient) ListTools() ([]Tool, error) {
	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	c.config.Logger.Println("Requesting tools list...")

	responseChan := make(chan *Response, 1)
	requestID := "tools-list"

	c.mu.Lock()
	c.responseHandlers[requestID] = responseChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.responseHandlers, requestID)
		c.mu.Unlock()
	}()

	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))
	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      &rawID,
		Params:  json.RawMessage(`{"cursor":""}`),
	}

	if err := c.sendMessage(&request); err != nil {
		return nil, fmt.Errorf("failed to send tools/list request: %v", err)
	}

	c.config.Logger.Println("Waiting for tools list response...")

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, fmt.Errorf("server error: %s (code: %d)",
				response.Error.Message, response.Error.Code)
		}

		c.config.Logger.Printf("Response Result: %+v", response.Result)

		resultBytes, err := json.Marshal(response.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %v", err)
		}

		var result ListToolsResult
		if err := json.Unmarshal(resultBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to parse tools list: %v", err)
		}

		c.config.Logger.Printf("Received %d tools from server", len(result.Tools))
		return result.Tools, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("tools/list request timeout")
	}
}

func (c *SSEClient) sendMessage(message interface{}) error {
	c.mu.RLock()
	endpoint := c.messageEndpoint
	c.mu.RUnlock()

	if endpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	c.config.Logger.Printf("Sending message: %s", string(jsonData))

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *SSEClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == Disconnected {
		return nil
	}

	close(c.stopChan)

	if c.eventStream != nil {
		c.eventStream.Body.Close()
		c.eventStream = nil
	}

	c.state = Disconnected
	c.initialized = false
	return nil
}

func (c *SSEClient) handlePing() {
	c.mu.Lock()
	c.lastPingTime = time.Now()
	c.missedPings = 0
	c.mu.Unlock()
	c.config.Logger.Println("Received ping from server")
}

func (c *SSEClient) monitorPings() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkPingStatus()
		}
	}
}

func (c *SSEClient) checkPingStatus() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastPingTime) > 2*pingInterval {
		c.missedPings++
		c.config.Logger.Printf("Missed ping #%d", c.missedPings)

		if c.missedPings >= defaultMaxMissedPings {
			c.config.Logger.Printf("Connection lost (missed %d pings)", c.missedPings)
			c.state = Disconnected
			c.initialized = false
		}
	}
}

func (c *SSEClient) extractClientID(endpoint string) string {
	parts := strings.Split(endpoint, "clientID=")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
