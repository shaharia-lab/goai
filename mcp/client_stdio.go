package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type StdIOClientConfig struct {
	RetryDelay    time.Duration
	MaxRetries    int
	ClientName    string
	ClientVersion string
	Logger        *log.Logger
	Reader        io.Reader
	Writer        io.Writer
}

type StdIOClient struct {
	config           StdIOClientConfig
	retryAttempt     int
	state            ConnectionState
	stopChan         chan struct{}
	initialized      bool
	capabilities     Capabilities
	protocolVersion  string
	mu               sync.RWMutex
	responseHandlers map[string]chan *Response
	nextRequestID    int
	reader           *bufio.Reader
	writer           io.Writer
}

func NewStdIOClient(config StdIOClientConfig) *StdIOClient {
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
	if config.Reader == nil {
		config.Reader = os.Stdin
	}
	if config.Writer == nil {
		config.Writer = os.Stdout
	}

	return &StdIOClient{
		config:           config,
		stopChan:         make(chan struct{}),
		state:            Disconnected,
		responseHandlers: make(map[string]chan *Response),
		nextRequestID:    1,
		reader:           bufio.NewReader(config.Reader),
		writer:           config.Writer,
	}
}

func (c *StdIOClient) Connect() error {
	c.mu.Lock()
	if c.state != Disconnected {
		c.mu.Unlock()
		return fmt.Errorf("client is already connected or connecting")
	}
	c.state = Connecting
	c.mu.Unlock()

	c.config.Logger.Println("Starting connection process...")

	go c.processIncomingMessages()

	if err := c.sendInitializeRequest(); err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}
	c.config.Logger.Println("Initialize request successful")

	if err := c.sendInitializedNotification(); err != nil {
		return fmt.Errorf("failed to send initialized notification: %v", err)
	}
	c.config.Logger.Println("Initialized notification sent successfully")

	c.mu.Lock()
	c.state = Connected
	c.initialized = true
	c.mu.Unlock()

	c.config.Logger.Println("Connection established successfully!")
	return nil
}

func (c *StdIOClient) processIncomingMessages() {
	scanner := bufio.NewScanner(c.reader)
	for {
		select {
		case <-c.stopChan:
			return
		default:
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil && !strings.Contains(err.Error(), "file already closed") {
					c.config.Logger.Printf("Scanner error: %v", err)
				}
				return
			}

			line := scanner.Text()
			c.config.Logger.Printf("Received raw input: %s", line)

			var response Response
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				c.config.Logger.Printf("Failed to parse response: %v", err)
				continue
			}

			if response.ID != nil {
				// Convert the RawMessage to string and remove quotes
				var idStr string
				if err := json.Unmarshal(*response.ID, &idStr); err != nil {
					c.config.Logger.Printf("Failed to unmarshal ID: %v", err)
					continue
				}

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
	}
}

func (c *StdIOClient) sendInitializeRequest() error {
	responseChan := make(chan *Response, 1)
	requestID := "init"

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

	// Create RawMessage for ID that matches our handler key
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

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

func (c *StdIOClient) sendInitializedNotification() error {
	notification := Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.sendMessage(&notification)
}

func (c *StdIOClient) ListTools() ([]Tool, error) {
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

		resultBytes, err := json.Marshal(response.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %v", err)
		}

		var result ListToolsResult
		if err := json.Unmarshal(resultBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to parse tools list: %v", err)
		}

		// Pretty print each tool's details
		for _, tool := range result.Tools {
			var schema map[string]interface{}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				c.config.Logger.Printf("Warning: Could not parse schema for tool %s: %v", tool.Name, err)
				continue
			}

			c.config.Logger.Printf("Tool: %s\n  Description: %s\n  Schema: %+v\n",
				tool.Name,
				tool.Description,
				schema,
			)
		}

		c.config.Logger.Printf("Found %d tools", len(result.Tools))
		return result.Tools, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("tools/list request timeout")
	}
}

func (c *StdIOClient) sendMessage(message interface{}) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	jsonData = append(jsonData, '\n')
	c.config.Logger.Printf("Sending message: %s", string(jsonData))

	_, err = c.writer.Write(jsonData)
	if err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}

	return nil
}

func (c *StdIOClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == Disconnected {
		return nil
	}

	c.config.Logger.Println("Shutting down client...")
	close(c.stopChan)
	c.state = Disconnected
	c.initialized = false
	c.config.Logger.Println("Client shutdown complete")
	return nil
}
