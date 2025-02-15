package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
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
	*BaseClient
	config          SSEClientConfig
	messageEndpoint string
	clientID        string
	lastPingTime    time.Time
	missedPings     int
	logger          *log.Logger
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
		BaseClient: NewBaseClient(),
		config:     config,
		logger:     config.Logger,
	}
}

func (c *SSEClient) Connect() error {
	if c.GetState() != Disconnected {
		return fmt.Errorf("client is already connected or connecting")
	}

	c.setState(Connecting)
	c.logger.Println("Starting connection process...")

	retryCount := 0
	for {
		err := c.establishConnection()
		if err == nil {
			c.resetRetryAttempt()
			return nil
		}

		retryCount++
		if retryCount >= c.config.MaxRetries {
			return fmt.Errorf("max retries reached: %v", err)
		}

		delay := c.calculateBackoff(retryCount)
		c.logger.Printf("Connection attempt failed: %v. Retrying in %v...", err, delay)
		time.Sleep(delay)
	}
}

func (c *SSEClient) establishConnection() error {
	c.logger.Printf("Connecting to SSE endpoint: %s", c.config.URL)
	resp, err := http.Get(c.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to events endpoint: %v", err)
	}

	scanner := bufio.NewScanner(resp.Body)
	messageEndpointChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		c.processEventStream(scanner, messageEndpointChan, errChan)
	}()

	select {
	case endpoint := <-messageEndpointChan:
		c.messageEndpoint = endpoint
		c.clientID = c.extractClientID(endpoint)
		c.logger.Printf("Received message endpoint: %s with client ID: %s", endpoint, c.clientID)
	case err := <-errChan:
		return fmt.Errorf("failed to get message endpoint: %v", err)
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for message endpoint")
	}

	if err := c.sendInitializeRequest(); err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}

	if err := c.sendInitializedNotification(); err != nil {
		return fmt.Errorf("failed to send initialized notification: %v", err)
	}

	c.setState(Connected)
	c.setInitialized(true)
	c.lastPingTime = time.Now()

	go c.monitorPings()

	c.logger.Println("Connection established successfully!")
	return nil
}

func (c *SSEClient) processEventStream(scanner *bufio.Scanner, messageEndpointChan chan string, errChan chan error) {
	defer c.logger.Println("Event stream processing stopped")

	var endpointSent bool
	for scanner.Scan() {
		select {
		case <-c.stopChan:
			return
		default:
			line := scanner.Text()
			c.logger.Printf("SSE Raw line: %s", line)

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
					if !endpointSent {
						messageEndpointChan <- data
						endpointSent = true
					}
					continue
				}

				var response Response
				if err := json.Unmarshal([]byte(data), &response); err != nil {
					c.logger.Printf("Failed to parse response: %v", err)
					continue
				}

				if response.ID != nil {
					idStr := strings.Trim(string(*response.ID), "\"")
					if ch, exists := c.getResponseHandler(idStr); exists {
						ch <- &response
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		errChan <- err
		c.setState(Disconnected)
		c.setInitialized(false)

		go func() {
			c.logger.Printf("Connection lost, attempting to reconnect...")
			if err := c.Connect(); err != nil {
				c.logger.Printf("Failed to reconnect: %v", err)
			}
		}()
	}
}

func (c *SSEClient) sendInitializeRequest() error {
	responseChan := make(chan *Response, 1)
	requestID := "init"
	rawID := json.RawMessage(`"init"`)

	c.addResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

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

	if err := c.sendMessage(&request); err != nil {
		return err
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		var result InitializeResult
		resultBytes, err := json.Marshal(response.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal response result: %v", err)
		}

		if err := json.Unmarshal(resultBytes, &result); err != nil {
			return fmt.Errorf("failed to parse initialize result: %v", err)
		}

		c.setCapabilities(result.Capabilities)
		c.setProtocolVersion(result.ProtocolVersion)

		c.logger.Printf("Server capabilities received: %+v", result.Capabilities)
		return nil

	case <-time.After(30 * time.Second):
		return fmt.Errorf("initialize request timeout")
	}
}

func (c *SSEClient) sendInitializedNotification() error {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.sendMessage(&notification)
}

func (c *SSEClient) ListTools() ([]Tool, error) {
	if c.GetState() != Connected || !c.IsInitialized() {
		return nil, fmt.Errorf("client is not connected and initialized")
	}

	c.logger.Println("Requesting tools list...")

	responseChan := make(chan *Response, 1)
	requestID := "tools-list"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.addResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      &rawID,
		Params:  json.RawMessage(`{"cursor":""}`),
	}

	if err := c.sendMessage(&request); err != nil {
		return nil, fmt.Errorf("failed to send tools/list request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		var result ListToolsResult
		resultBytes, err := json.Marshal(response.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %v", err)
		}

		if err := json.Unmarshal(resultBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to parse tools list: %v", err)
		}

		c.logger.Printf("Received %d tools from server", len(result.Tools))
		return result.Tools, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("tools/list request timeout")
	}
}

func (c *SSEClient) sendMessage(message interface{}) error {
	if c.messageEndpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	c.logger.Printf("Sending message: %s", string(jsonData))

	req, err := http.NewRequest(http.MethodPost, c.messageEndpoint, bytes.NewBuffer(jsonData))
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
	if c.GetState() == Disconnected {
		return nil
	}

	close(c.stopChan)
	c.setState(Disconnected)
	c.setInitialized(false)
	return nil
}

func (c *SSEClient) handlePing() {
	c.mu.Lock()
	c.lastPingTime = time.Now()
	c.missedPings = 0
	c.mu.Unlock()
	c.logger.Println("Received ping from server")
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
		c.logger.Printf("Missed ping #%d", c.missedPings)

		if c.missedPings >= defaultMaxMissedPings {
			c.logger.Printf("Connection lost (missed %d pings)", c.missedPings)
			c.state = Disconnected
			c.initialized = false

			go func() {
				c.logger.Println("Attempting to reconnect...")
				if err := c.Connect(); err != nil {
					c.logger.Printf("Failed to reconnect: %v", err)
				}
			}()
		}
	}
}

func (c *SSEClient) calculateBackoff(attempt int) time.Duration {
	baseDelay := c.config.RetryDelay
	maxDelay := defaultMaxRetryDelay

	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}

	jitter := 0.1
	backoff = backoff * (1 + jitter*(2*rand.Float64()-1))

	return time.Duration(backoff)
}

func (c *SSEClient) extractClientID(endpoint string) string {
	parts := strings.Split(endpoint, "clientID=")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
