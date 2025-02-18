package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SSETransport struct {
	client          *http.Client
	config          SSEConfig
	logger          *log.Logger
	stopChan        chan struct{}
	mu              sync.RWMutex
	lastPingTime    time.Time
	missedPings     int
	messageEndpoint string
	receiveCallback func(message []byte)
	clientID        string
	state           ConnectionState
}

func NewSSETransport() *SSETransport {
	return &SSETransport{
		client:          &http.Client{},
		stopChan:        make(chan struct{}),
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
	t.state = Connecting
	retryCount := 0

	for {
		if err := t.establishConnection(ctx, config); err != nil {
			retryCount++
			if retryCount >= config.MaxRetries {
				return fmt.Errorf("max retries reached: %v", err)
			}

			delay := calculateBackoff(config.RetryDelay, retryCount)
			t.logger.Printf("Connection attempt failed: %v, Retrying in %v...", err, delay)
			select {
			case <-time.After(delay):
			case <-t.stopChan:
				return fmt.Errorf("connection cancelled")
			}
		} else {
			t.state = Connected
			return nil
		}
	}
}

func (t *SSETransport) establishConnection(ctx context.Context, config ClientConfig) error {
	t.logger.Printf("Connecting to SSE endpoint: %s", t.config.URL)
	resp, err := http.NewRequestWithContext(ctx, "GET", t.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to events endpoint: %v", err)
	}

	scanner := bufio.NewScanner(resp.Body)
	messageEndpointChan := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		defer resp.Body.Close()
		t.processEventStream(ctx, scanner, messageEndpointChan, errChan, config)
	}()

	select {
	case endpoint := <-messageEndpointChan:
		t.messageEndpoint = endpoint
		t.clientID = t.extractClientID(endpoint)
		config.Logger.Printf("Received message endpoint: %s, client ID: %s", t.messageEndpoint, t.clientID)
	case err := <-errChan:
		t.state = Disconnected
		return fmt.Errorf("failed to get message endpoint: %v", err)
	case <-time.After(30 * time.Second):
		t.state = Disconnected
		return fmt.Errorf("timeout waiting for message endpoint")
	}

	t.lastPingTime = time.Now()
	go t.monitorPings(ctx)

	return nil
}

func (t *SSETransport) processEventStream(ctx context.Context, scanner *bufio.Scanner, messageEndpointChan chan string, errChan chan error, config ClientConfig) {
	defer config.Logger.Println("Event stream processing stopped")

	var endpointSent bool
	for scanner.Scan() {
		select {
		case <-t.stopChan:
			return
		default:
			line := scanner.Text()
			config.Logger.Printf("SSE Raw line: %s", line)

			if line == "" {
				continue
			}

			if line == ":ping" {
				t.handlePing(ctx)
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

				t.receiveCallback([]byte(data))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		errChan <- err
		t.state = Disconnected
	}
}

func (t *SSETransport) SendMessage(ctx context.Context, message interface{}) error {
	if t.messageEndpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	t.logger.Printf("Sending message: %s", string(jsonData))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.messageEndpoint, bytes.NewBuffer(jsonData))
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

func (t *SSETransport) Close(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state == Disconnected {
		return nil
	}

	select {
	case <-t.stopChan:
		// Already closed
	default:
		close(t.stopChan)
	}

	t.setState(Disconnected)
	return nil
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
			t.checkPingStatus(ctx)
		}
	}
}

func (t *SSETransport) checkPingStatus(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Since(t.lastPingTime) > 2*pingInterval {
		t.missedPings++
		t.logger.Printf("Missed ping #%d", t.missedPings)

		if t.missedPings >= defaultMaxMissedPings {
			t.logger.Printf("Connection lost (missed %d pings)", t.missedPings)
			t.state = Disconnected
		}
	}
}

func (t *SSETransport) extractClientID(endpoint string) string {
	parts := strings.Split(endpoint, "clientID=")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
