package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

const (
	defaultBaseRetryDelay  = 5 * time.Second
	defaultMaxRetryDelay   = 30 * time.Second
	defaultMaxRetries      = 5
	defaultMaxMissedPings  = 2
	defaultClientName      = "mcp-client"
	defaultClientVersion   = "1.0.0"
	defaultMessageEndpoint = ""
	defaultRequestTimeout  = 30 * time.Second
)

type ConnectionState int

const (
	Disconnected ConnectionState = iota
	Connecting
	Connected
)

type SSEConfig struct {
	URL string
}

type StdIOConfig struct {
	Reader io.Reader
	Writer io.Writer
}

type ClientConfig struct {
	RetryDelay      time.Duration
	MaxRetries      int
	ClientName      string
	ClientVersion   string
	Logger          *log.Logger
	SSE             SSEConfig
	StdIO           StdIOConfig
	MessageEndpoint string
	RequestTimeout  time.Duration
}

type Transport interface {
	Connect(ctx context.Context, config ClientConfig) error
	SendMessage(ctx context.Context, message interface{}) error
	Close(ctx context.Context) error
	SetReceiveMessageCallback(ctx context.Context, callback func(message []byte))
}

type Client struct {
	transport        Transport
	config           ClientConfig
	logger           *log.Logger
	state            ConnectionState
	initialized      bool
	capabilities     Capabilities
	protocolVersion  string
	mu               sync.RWMutex
	responseHandlers map[string]chan *Response
	wg               sync.WaitGroup
}

func NewClient(transport Transport, config ClientConfig) *Client {
	if config.Logger == nil {
		config.Logger = log.Default()
	}
	if config.ClientName == "" {
		config.ClientName = defaultClientName
	}
	if config.ClientVersion == "" {
		config.ClientVersion = defaultClientVersion
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = defaultBaseRetryDelay
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = defaultMaxRetries
	}

	client := &Client{
		transport:        transport,
		config:           config,
		logger:           config.Logger,
		state:            Disconnected,
		capabilities:     Capabilities{},
		responseHandlers: make(map[string]chan *Response),
	}

	transport.SetReceiveMessageCallback(context.Background(), client.processReceivedMessage)
	return client
}

func (c *Client) registerResponseHandler(id string, ch chan *Response) {
	c.mu.Lock()
	c.responseHandlers[id] = ch
	c.mu.Unlock()
}

func (c *Client) removeResponseHandler(id string) {
	c.mu.Lock()
	delete(c.responseHandlers, id)
	c.mu.Unlock()
}

func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.state != Disconnected {
		c.mu.Unlock()
		return fmt.Errorf("client is already connected or connecting")
	}
	c.state = Connecting
	c.mu.Unlock()

	c.logger.Println("Starting connection process...")

	if err := c.transport.Connect(ctx, c.config); err != nil {
		return err
	}
	c.logger.Printf("Transport connected successfully")

	if err := c.sendInitializeRequest(ctx); err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}
	c.logger.Println("Initialize request successful")

	if err := c.sendInitializedNotification(ctx); err != nil {
		return fmt.Errorf("failed to send initialized notification: %v", err)
	}

	c.logger.Println("Initialized notification sent")

	c.mu.Lock()
	c.state = Connected
	c.initialized = true
	c.mu.Unlock()
	c.logger.Println("Connection established successfully!")

	return nil
}

func (c *Client) processReceivedMessage(message []byte) {
	log.Printf("Received message: %s (length: %d)", string(message), len(message))
	var response Response
	if err := json.Unmarshal(message, &response); err != nil {
		c.logger.Printf("Failed to parse response: %v", err)
		return
	}

	if response.ID != nil {
		var idStr string
		if err := json.Unmarshal(*response.ID, &idStr); err != nil {
			c.logger.Printf("Failed to unmarshal ID: %v", err)
			return
		}

		c.logger.Printf("Processing response for request ID: %s", idStr)
		c.mu.RLock()
		ch, exists := c.responseHandlers[idStr]
		c.mu.RUnlock()

		if exists {
			select {
			case ch <- &response:
				c.logger.Printf("Response sent to handler for request ID: %s", idStr)
			default:
				c.logger.Printf("Handler channel full for request ID: %s", idStr)
			}
		} else {
			c.logger.Printf("No handler found for request ID: %s", idStr)
		}
	}
}

func (c *Client) sendInitializeRequest(ctx context.Context) error {
	responseChan := make(chan *Response, 1)
	requestID := "init"
	rawID := json.RawMessage(`"init"`)

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)

	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]any{
			"tools": map[string]bool{
				"listChanged": true,
			},
		},
		ClientInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    c.config.ClientName,
			Version: c.config.ClientVersion,
		},
	}

	request := Request{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      &rawID,
		Params:  nil,
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		c.wg.Done()
		return fmt.Errorf("failed to marshal initialize params: %v", err)
	}
	request.Params = paramsBytes

	if err := c.transport.SendMessage(ctx, &request); err != nil {
		c.wg.Done()
		return err
	}

	select {
	case genericResponse := <-responseChan:
		defer c.wg.Done()
		if genericResponse.Error != nil {
			return fmt.Errorf("server error: %s (code: %d)", genericResponse.Error.Message, genericResponse.Error.Code)
		}

		rawResult, err := json.Marshal(genericResponse.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var initResponse InitializeResponse
		if err := json.Unmarshal(rawResult, &initResponse); err != nil {
			return fmt.Errorf("failed to unmarshal raw message into initialize response: %v", err)
		}

		c.mu.Lock()
		c.capabilities = initResponse.Result.Capabilities
		c.protocolVersion = initResponse.Result.ProtocolVersion
		c.mu.Unlock()

		c.logger.Printf("Server capabilities received: %+v", c.capabilities)
		return nil

	case <-time.After(30 * time.Second):
		defer c.wg.Done()
		return fmt.Errorf("initialize request timeout")
	}
}

func (c *Client) sendInitializedNotification(ctx context.Context) error {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.transport.SendMessage(ctx, &notification)
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	c.logger.Println("Requesting tools list...")

	responseChan := make(chan *Response, 1)
	requestID := "tools-list"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)

	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      &rawID,
		Params:  json.RawMessage(`{"cursor":""}`),
	}

	if err := c.transport.SendMessage(ctx, &request); err != nil {
		c.wg.Done()
		return nil, fmt.Errorf("failed to send tools/list request: %v", err)
	}

	select {
	case response := <-responseChan:
		defer c.wg.Done()
		if response.Error != nil {
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result ListToolsResult
		if err := json.Unmarshal(rawResult, &result); err != nil {
			return nil, fmt.Errorf("failed to parse tools list: %v", err)
		}
		return result.Tools, nil

	case <-time.After(30 * time.Second):
		defer c.wg.Done()
		return nil, fmt.Errorf("tools/list request timeout")
	}
}

func (c *Client) CallTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return CallToolResult{}, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	c.logger.Println("Calling tool...")

	responseChan := make(chan *Response, 1)
	requestID := "tools-call"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)
	defer c.wg.Done()

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal CallToolParams: %v", err)
	}

	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      &rawID,
		Params:  json.RawMessage(paramsBytes),
	}

	// Create a timeout context for this request
	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	if err := c.transport.SendMessage(requestCtx, &request); err != nil {
		return CallToolResult{}, fmt.Errorf("failed to send tools/call request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return CallToolResult{}, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result CallToolResult
		if err := json.Unmarshal(rawResult, &result); err != nil {
			return CallToolResult{}, fmt.Errorf("failed to parse tool result: %v", err)
		}
		return result, nil

	case <-requestCtx.Done():
		return CallToolResult{}, fmt.Errorf("tools/call request timeout: %v", requestCtx.Err())
	}
}

func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	c.logger.Println("Requesting prompts list...")

	responseChan := make(chan *Response, 1)
	requestID := "prompts-list"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)
	defer c.wg.Done()

	request := Request{
		JSONRPC: "2.0",
		Method:  "prompts/list",
		ID:      &rawID,
		Params:  json.RawMessage(`{"cursor":""}`),
	}

	// Create a timeout context for this request
	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	if err := c.transport.SendMessage(requestCtx, &request); err != nil {
		return nil, fmt.Errorf("failed to send prompts/list request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result ListPromptsResult
		if err := json.Unmarshal(rawResult, &result); err != nil {
			return nil, fmt.Errorf("failed to parse prompts list: %v", err)
		}
		return result.Prompts, nil

	case <-requestCtx.Done():
		return nil, fmt.Errorf("prompts/list request timeout: %v", requestCtx.Err())
	}
}

func (c *Client) GetPrompt(ctx context.Context, params GetPromptParams) ([]PromptMessage, error) {
	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	c.logger.Println("Requesting prompt details...")

	responseChan := make(chan *Response, 1)
	requestID := "prompts-get"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)
	defer c.wg.Done()

	paramsBytes, _ := json.Marshal(params)
	request := Request{
		JSONRPC: "2.0",
		Method:  "prompts/get",
		ID:      &rawID,
		Params:  json.RawMessage(paramsBytes),
	}

	// Create a timeout context for this request
	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	if err := c.transport.SendMessage(requestCtx, &request); err != nil {
		return nil, fmt.Errorf("failed to send prompts/get request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result PromptGetResponse
		if err := json.Unmarshal(rawResult, &result); err != nil {
			return nil, fmt.Errorf("failed to parse prompt details: %v", err)
		}
		return result.Messages, nil

	case <-requestCtx.Done():
		return nil, fmt.Errorf("prompts/get request timeout: %v", requestCtx.Err())
	}
}

func (c *Client) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.state == Disconnected {
		c.mu.Unlock()
		return nil
	}

	c.logger.Println("Shutting down client...")

	c.wg.Wait()

	if err := c.transport.Close(ctx); err != nil {
		c.mu.Unlock()
		return err
	}

	c.state = Disconnected
	c.initialized = false
	c.mu.Unlock()
	c.logger.Println("Client shutdown complete")
	return nil
}

func (c *Client) GetState() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Client) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

func (c *Client) GetCapabilities() Capabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capabilities
}

func (c *Client) GetProtocolVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolVersion
}

func calculateBackoff(baseDelay time.Duration, attempt int) time.Duration {
	maxDelay := defaultMaxRetryDelay

	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}

	jitter := 0.1
	backoff = backoff * (1 + jitter*(2*rand.Float64()-1))

	return time.Duration(backoff)
}
