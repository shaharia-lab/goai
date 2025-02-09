package mcp

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type reader struct {
	data []byte
	pos  int
	done bool
}

func NewReader(data []byte) *reader {
	return &reader{
		data: data,
		pos:  0,
		done: false,
	}
}

type SSEServer struct {
	server     *StdIOServer
	clients    map[string]*sseClient // Changed client type
	clientsMu  sync.RWMutex
	logger     *log.Logger
	httpServer *http.Server
}

type sseClient struct {
	writer   http.ResponseWriter
	flusher  http.Flusher
	messages chan []byte
}

func NewSSEServer(mcpServer *StdIOServer, addr string, logger *log.Logger) *SSEServer {
	s := &SSEServer{
		server:  mcpServer,
		clients: make(map[string]*sseClient),
		logger:  logger,
	}

	mux := http.NewServeMux()

	// Handle /sse endpoint explicitly
	mux.HandleFunc("/sse", s.handleEvents)

	// Handle /client/ endpoints
	mux.HandleFunc("/client/", s.handleClientMessage)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return s
}

// Add this new method to SSEServer
func (s *SSEServer) Start(ctx context.Context) error {
	// Start the HTTP server in a goroutine
	go func() {
		s.logger.Printf("Starting SSE server on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Cleanup all clients
	s.clientsMu.Lock()
	for id, client := range s.clients {
		close(client.messages)
		delete(s.clients, id)
	}
	s.clientsMu.Unlock()

	// Gracefully shutdown the server
	return s.httpServer.Shutdown(shutdownCtx)
}

func (s *SSEServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientID := uuid.NewString()
	client := &sseClient{
		writer:   w,
		flusher:  flusher,
		messages: make(chan []byte, 100),
	}

	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.clientsMu.Unlock()

	endpointURL := fmt.Sprintf("/client/%s/messages", clientID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL)
	flusher.Flush()

	ctx := r.Context()
	go s.messagePump(ctx, client)

	<-ctx.Done()

	s.clientsMu.Lock()
	if client, ok := s.clients[clientID]; ok {
		close(client.messages)
		delete(s.clients, clientID)
	}
	s.clientsMu.Unlock()
}

func (s *SSEServer) handleClientMessage(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 || parts[1] != "client" || parts[3] != "messages" {
		http.NotFound(w, r)
		return
	}
	clientID := parts[2]

	s.clientsMu.RLock()
	client, exists := s.clients[clientID]
	s.clientsMu.RUnlock()

	if !exists {
		http.NotFound(w, r)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer cancel()

		writer := &SSEWriter{client: client}
		reader := NewReader(body)
		requestServer := NewStdIOServer(reader, writer)
		if err := requestServer.Run(ctx); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				s.logger.Printf("Error processing message: %v", err)
			}
		}
	}()

	// Wait for processing to complete or timeout
	go func() {
		wg.Wait()
		cancel()
	}()

	w.WriteHeader(http.StatusAccepted)
}

type SSEWriter struct {
	client *sseClient
}

func (w *SSEWriter) Write(p []byte) (n int, err error) {
	w.client.messages <- p
	return len(p), nil
}

func (s *SSEServer) handleClientMessageWrapper(w http.ResponseWriter, r *http.Request) {
	// Extract clientID from the URL path.  MUST match the endpoint URL format.
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 || parts[1] != "client" || parts[3] != "messages" {
		s.logger.Printf("Invalid URL path: %s", r.URL.Path)
		http.NotFound(w, r)
		return
	}
	clientID := parts[2]

	// Verify the client exists (using Read Lock for concurrency safety).
	s.clientsMu.RLock()
	_, exists := s.clients[clientID]
	s.clientsMu.RUnlock()

	if !exists {
		s.logger.Printf("Client not found: %s", clientID)
		http.NotFound(w, r)
		return
	}

	s.handleClientMessage(w, r)
}

func (r *reader) Read(p []byte) (n int, err error) {
	if r.done {
		return 0, io.EOF
	}

	if r.pos >= len(r.data) {
		r.done = true
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n

	if r.pos >= len(r.data) {
		r.done = true
	}

	return n, nil
}

// Add this method to your SSEServer type
// Add to sse_server.go

func (s *SSEServer) Run(ctx context.Context) error {
	// Create a server shutdown context
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	// Start the HTTP server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
			serverCancel()
		}
	}()

	// Wait for context cancellation
	<-serverCtx.Done()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Cleanup all clients
	s.clientsMu.Lock()
	for id, client := range s.clients {
		close(client.messages)
		delete(s.clients, id)
	}
	s.clientsMu.Unlock()

	// Gracefully shutdown the server
	return s.httpServer.Shutdown(shutdownCtx)
}

func (s *SSEServer) messagePump(ctx context.Context, client *sseClient) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.messages:
			if !ok {
				return
			}
			fmt.Fprintf(client.writer, "data: %s\n\n", msg)
			client.flusher.Flush()
		}
	}
}
