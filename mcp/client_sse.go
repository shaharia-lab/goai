package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	//pingInterval       = 30 * time.Second
	defaultPingTimeout = 2 * pingInterval
)

type SSETransport struct {
	client          *http.Client
	config          SSEConfig
	logger          *log.Logger
	stopChan        chan struct{}
	reconnectChan   chan struct{}
	mu              sync.RWMutex
	lastPingTime    time.Time
	missedPings     int
	messageEndpoint string
	receiveCallback func(message []byte)
	clientID        string
	state           ConnectionState
	connOnce        sync.Once
	closeOnce       sync.Once
}

func NewSSETransport() *SSETransport {
	return &SSETransport{
		client:          &http.Client{},
		stopChan:        make(chan struct{}),
		reconnectChan:   make(chan struct{}, 1),
		logger:          log.Default(),
		messageEndpoint: defaultMessageEndpoint,
		state:           Disconnected,
	}
}

func (t *SSETransport) SetReceiveMessageCallback(ctx context.Context, callback func(message []byte)) {
	t.receiveCallback = callback
}

func (t *SSETransport) Connect(ctx context.Context, config ClientConfig) error {
	t.config = config.SSE
	t.logger = config.Logger
	t.messageEndpoint = config.MessageEndpoint
	t.setState(Connecting)

	// Reset connection state
	t.connOnce = sync.Once{}

	// Start connection manager
	t.connOnce.Do(func() {
		go t.connectionManager(ctx, config)
	})

	// Wait for initial connection or failure
	select {
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for initial connection")
	case <-ctx.Done():
		return ctx.Err()
	case <-t.reconnectChan:
		// Connection succeeded
		return nil
	}
}

func (t *SSETransport) connectionManager(ctx context.Context, config ClientConfig) {
	retryCount := 0

	for {
		select {
		case <-t.stopChan:
			return
		default:
			if t.getState() != Connecting && t.getState() != Disconnected {
				// Wait for disconnect or reconnect signal
				select {
				case <-t.stopChan:
					return
				case <-t.reconnectChan:
					t.setState(Connecting)
				}
			}

			if err := t.establishConnection(ctx, config); err != nil {
				retryCount++
				if retryCount >= config.MaxRetries {
					t.logger.Printf("Max retries reached: %v", err)
					t.setState(Disconnected)
					return
				}

				delay := calculateBackoff(config.RetryDelay, retryCount)
				t.logger.Printf("Connection attempt failed: %v, Retrying in %v...", err, delay)
				select {
				case <-time.After(delay):
				case <-t.stopChan:
					return
				}
			} else {
				// Connection established successfully
				t.setState(Connected)
				retryCount = 0

				// Notify about successful connection
				select {
				case t.reconnectChan <- struct{}{}:
				default:
					// Channel is full, that's OK
				}
			}
		}
	}
}

func (t *SSETransport) establishConnection(ctx context.Context, config ClientConfig) error {
	t.logger.Printf("Connecting to SSE endpoint: %s", t.config.URL)

	req, err := http.NewRequestWithContext(ctx, "GET", t.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Add Accept header for SSE
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to events endpoint: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	messageEndpointChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		t.processEventStream(ctx, resp.Body, messageEndpointChan, errChan, config)
	}()

	select {
	case endpoint := <-messageEndpointChan:
		t.messageEndpoint = endpoint
		t.clientID = t.extractClientID(endpoint)
		config.Logger.Printf("Received message endpoint: %s, client ID: %s", t.messageEndpoint, t.clientID)
	case err := <-errChan:
		t.setState(Disconnected)
		return fmt.Errorf("failed to get message endpoint: %v", err)
	case <-time.After(30 * time.Second):
		t.setState(Disconnected)
		return fmt.Errorf("timeout waiting for message endpoint")
	case <-ctx.Done():
		return ctx.Err()
	}

	t.mu.Lock()
	t.lastPingTime = time.Now()
	t.missedPings = 0
	t.mu.Unlock()

	go t.monitorPings(ctx)

	return nil
}

func (t *SSETransport) processEventStream(ctx context.Context, reader io.Reader, messageEndpointChan chan string, errChan chan error, config ClientConfig) {
	defer config.Logger.Println("Event stream processing stopped")

	var endpointSent bool
	buffer := make([]byte, 1024*1024)

	event := make(map[string]string)
	dataBuffer := new(bytes.Buffer)

	for {
		select {
		case <-t.stopChan:
			return
		default:
			// Read data into buffer
			n, err := reader.Read(buffer)
			if err != nil {
				if err != io.EOF {
					errChan <- fmt.Errorf("error reading from event stream: %v", err)
				} else {
					errChan <- fmt.Errorf("event stream closed unexpectedly")
				}

				// Signal reconnect
				select {
				case t.reconnectChan <- struct{}{}:
				default:
					// Channel is full, that's OK
				}
				return
			}

			// Process the bytes read
			data := buffer[:n]
			lines := bytes.Split(data, []byte("\n"))

			for _, line := range lines {
				lineStr := string(line)

				// Empty line marks the end of an event
				if lineStr == "" {
					if len(event) > 0 {
						t.processEvent(event, dataBuffer, messageEndpointChan, endpointSent, &endpointSent)
						// Clear for next event
						event = make(map[string]string)
						dataBuffer.Reset()
					}
					continue
				}

				// Handle ping event
				if lineStr == ":ping" {
					t.handlePing(ctx)
					continue
				}

				// Parse event fields
				if strings.HasPrefix(lineStr, "data:") {
					data := strings.TrimPrefix(lineStr, "data:")
					if len(data) > 0 && data[0] == ' ' {
						data = data[1:] // Remove space if it exists
					}
					dataBuffer.WriteString(data)
					dataBuffer.WriteString("\n")
					event["data"] = dataBuffer.String()
				} else if strings.HasPrefix(lineStr, "event:") {
					event["event"] = strings.TrimSpace(strings.TrimPrefix(lineStr, "event:"))
				} else if strings.HasPrefix(lineStr, "id:") {
					event["id"] = strings.TrimSpace(strings.TrimPrefix(lineStr, "id:"))
				} else if strings.HasPrefix(lineStr, "retry:") {
					event["retry"] = strings.TrimSpace(strings.TrimPrefix(lineStr, "retry:"))
				}
			}
		}
	}
}

func (t *SSETransport) processEvent(event map[string]string, dataBuffer *bytes.Buffer,
	messageEndpointChan chan string, endpointSent bool, endpointSentPtr *bool) {

	if data, ok := event["data"]; ok {
		// Remove last newline if exists
		data = strings.TrimSuffix(data, "\n")

		// Check if data is a URL for message endpoint
		if (strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://")) && !endpointSent {
			messageEndpointChan <- data
			*endpointSentPtr = true
			return
		}

		// If we have a callback and this isn't an endpoint URL, process the data
		if t.receiveCallback != nil {
			t.receiveCallback([]byte(data))
		}
	}
}

func (t *SSETransport) SendMessage(ctx context.Context, message interface{}) error {
	t.mu.RLock()
	endpoint := t.messageEndpoint
	t.mu.RUnlock()

	if endpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	t.logger.Printf("Sending message: %s", string(jsonData))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// Check if the error was due to context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("failed to send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (t *SSETransport) Close(ctx context.Context) error {
	var err error

	t.closeOnce.Do(func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		if t.state == Disconnected {
			return
		}

		t.logger.Println("Closing SSE transport...")
		close(t.stopChan)
		t.state = Disconnected
	})

	return err
}

func (t *SSETransport) setState(s ConnectionState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = s
}

func (t *SSETransport) getState() ConnectionState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

func (t *SSETransport) handlePing(context.Context) {
	t.mu.Lock()
	t.lastPingTime = time.Now()
	t.missedPings = 0
	t.mu.Unlock()
	t.logger.Println("Received ping from server")
}

func (t *SSETransport) monitorPings(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopChan:
			return
		case <-ticker.C:
			if t.checkPingStatus(ctx) {
				// Signal reconnect
				select {
				case t.reconnectChan <- struct{}{}:
				default:
					// Channel is full, that's OK
				}
				return
			}
		}
	}
}

func (t *SSETransport) checkPingStatus(ctx context.Context) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Since(t.lastPingTime) > defaultPingTimeout {
		t.missedPings++
		t.logger.Printf("Missed ping #%d", t.missedPings)

		if t.missedPings >= defaultMaxMissedPings {
			t.logger.Printf("Connection lost (missed %d pings)", t.missedPings)
			t.state = Disconnected
			return true // Reconnect needed
		}
	}

	return false
}

func (t *SSETransport) extractClientID(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		t.logger.Printf("Failed to parse endpoint URL: %v", err)
		return ""
	}

	values := u.Query()
	return values.Get("clientID")
}
