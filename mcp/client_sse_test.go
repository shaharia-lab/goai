package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSSEClient_ConnectionFlow(t *testing.T) {
	tests := []struct {
		name           string
		serverBehavior func(w http.ResponseWriter, r *http.Request)
		expectedError  bool
	}{
		{
			name: "successful_connection_flow",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				flusher, ok := w.(http.Flusher)
				if !ok {
					t.Fatal("Expected ResponseWriter to be a Flusher")
				}

				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")

				// Create message endpoint server
				messageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{
						"jsonrpc": "2.0",
						"result": {
							"protocolVersion": "2024-11-05",
							"capabilities": {
								"tools": {"listChanged": true}
							}
						}
					}`))
				}))
				defer messageServer.Close()

				// Send the initial event with actual message endpoint
				_, _ = w.Write([]byte("event: connect\n"))
				_, _ = w.Write([]byte("data: " + messageServer.URL + "\n\n"))
				flusher.Flush()

				// Simulate server sending periodic pings
				for i := 0; i < 3; i++ {
					_, _ = w.Write([]byte(":ping\n\n"))
					flusher.Flush()
					time.Sleep(100 * time.Millisecond)
				}
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverBehavior))
			defer server.Close()

			stopChan := make(chan struct{})
			reconnectChan := make(chan struct{})
			client := NewSSEClient(SSEClientConfig{
				URL:           server.URL,
				RetryDelay:    100 * time.Millisecond,
				StopChan:      stopChan,
				ReconnectChan: reconnectChan,
			})

			// Start client in goroutine
			errChan := make(chan error, 1)
			go func() {
				client.Start()
				close(errChan)
			}()

			// Let it run for a short while
			time.Sleep(500 * time.Millisecond)

			// Stop client
			client.Stop()
		})
	}
}

func TestSSEClient_RetryMechanism(t *testing.T) {
	type serverResponse struct {
		statusCode int
		delay      time.Duration
		body       string
	}

	// Create message endpoint server first
	messageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
            "jsonrpc": "2.0",
            "result": {
                "protocolVersion": "2024-11-05",
                "capabilities": {
                    "tools": {"listChanged": true}
                }
            }
        }`))
	}))
	defer messageServer.Close()

	tests := []struct {
		name            string
		serverResponses []serverResponse
		expectedRetries int
		timeout         time.Duration
	}{
		{
			name: "successful_after_retries",
			serverResponses: []serverResponse{
				{
					statusCode: http.StatusInternalServerError,
					delay:      0,
				},
				{
					statusCode: http.StatusInternalServerError,
					delay:      0,
				},
				{
					statusCode: http.StatusOK,
					body:       fmt.Sprintf("event: connect\ndata: %s\n\n", messageServer.URL),
				},
			},
			expectedRetries: 2,
			timeout:         30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				responseIndex = 0
				retryCount    = 0
				mu            sync.Mutex
			)

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				currentIndex := responseIndex
				if currentIndex < len(tt.serverResponses) {
					responseIndex++
				}
				mu.Unlock()

				if currentIndex < len(tt.serverResponses) {
					response := tt.serverResponses[currentIndex]

					if response.delay > 0 {
						time.Sleep(response.delay)
					}

					if response.statusCode != http.StatusOK {
						mu.Lock()
						retryCount++
						mu.Unlock()
						w.WriteHeader(response.statusCode)
						return
					}

					// Success case
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					w.Header().Set("Connection", "keep-alive")
					_, _ = w.Write([]byte(response.body))
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}

					// Keep connection open
					<-r.Context().Done()
				}
			}))
			defer server.Close()

			// Create client with testing configuration
			stopChan := make(chan struct{})
			reconnectChan := make(chan struct{})

			client := NewSSEClient(SSEClientConfig{
				URL:           server.URL,
				RetryDelay:    time.Second, // Use a shorter delay for testing
				StopChan:      stopChan,
				ReconnectChan: reconnectChan,
				MaxRetries:    3,
			})

			// Create context with timeout
			_, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			// Start client in goroutine
			go func() {
				client.Start()
			}()

			// Wait for expected number of retries or timeout
			deadline := time.After(tt.timeout)
			for {
				mu.Lock()
				currentRetries := retryCount
				mu.Unlock()

				if currentRetries == tt.expectedRetries {
					close(stopChan)
					return
				}

				select {
				case <-deadline:
					close(stopChan)
					t.Errorf("Test timed out. Got %d retries, expected %d", currentRetries, tt.expectedRetries)
					return
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}
		})
	}
}
