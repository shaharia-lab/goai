package mcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SSEServer handles MCP communication over HTTP with StdIOServer-Sent Events (SSE).
type SSEServer struct {
	server     *StdIOServer            // The core MCP server
	clients    map[string]*http.Client // Store http clients.
	clientsMu  sync.RWMutex
	logger     *log.Logger
	httpServer *http.Server
}

// NewSSEServer creates a new SSEServer.
func NewSSEServer(mcpServer *StdIOServer, addr string, logger *log.Logger) *SSEServer {
	if logger == nil {
		logger = log.New(io.Discard, "", 0) // Default to no-op logger
	}
	s := &SSEServer{
		server:  mcpServer,
		clients: make(map[string]*http.Client),
		logger:  logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.handleEvents)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return s
}

// handleEvents is the StdIOServer-Sent Events endpoint.
func (s *SSEServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Consider restricting in production

	clientID := uuid.NewString()
	s.clientsMu.Lock()
	s.clients[clientID] = &http.Client{} // You can customize client as needed.
	s.clientsMu.Unlock()

	// Send the endpoint event immediately.
	endpointURL := fmt.Sprintf("/client/%s/messages", clientID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL)
	flusher.Flush()

	s.httpServer.Handler.(*http.ServeMux).HandleFunc(endpointURL, func(w http.ResponseWriter, r *http.Request) {
		s.handleClientMessage(w, r, clientID) // Pass clientID
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Clean up when the connection closes.
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, clientID) // Remove the client.
		s.clientsMu.Unlock()
		s.httpServer.Handler.(*http.ServeMux).Handle(endpointURL, http.NotFoundHandler()) // Remove handler
		s.logger.Printf("Client disconnected: %s", clientID)
	}()

	s.logger.Printf("Client connected: %s, endpoint: %s", clientID, endpointURL)

	for { // Keep the connection alive, sending pings.
		select {
		case <-ctx.Done():
			return // Client disconnected.
		case <-time.After(30 * time.Second): // Adjust interval as needed.
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n") // Send a ping event.
			flusher.Flush()
		}
	}
}

// handleClientMessage handles POST requests from the client.
func (s *SSEServer) handleClientMessage(w http.ResponseWriter, r *http.Request, clientID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading body", http.StatusBadRequest)
		return
	}

	s.logger.Printf("Received message from client %s: %s", clientID, string(body))

	// Create a reader for the message
	reader := NewReader(body)

	// Create a new response writer
	writer := &HTTPResponseWriter{
		httpWriter: w,
		logger:     s.logger,
	}

	// Create a server instance for this request
	requestServer := NewStdIOServer(reader, writer)

	// Process the message using server's Run method
	ctx := r.Context()
	if err := requestServer.Run(ctx); err != nil {
		s.logger.Printf("Error processing message: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HTTPResponseWriter implements necessary interfaces to write responses
type HTTPResponseWriter struct {
	httpWriter http.ResponseWriter
	logger     *log.Logger
}

func (w *HTTPResponseWriter) Write(p []byte) (n int, err error) {
	return w.httpWriter.Write(p)
}

// Run starts the HTTP server.  This should be run in a goroutine.
func (s *SSEServer) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.logger.Println("Shutting down HTTP server...")
		s.clientsMu.Lock()
		for clientID, client := range s.clients {
			s.logger.Printf("Closing client: %s", clientID)
			client.CloseIdleConnections() // Close the client connection.
			delete(s.clients, clientID)
		}
		s.clientsMu.Unlock()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	s.logger.Printf("HTTP server listening on %s", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server listen error: %w", err)
	}
	return nil
}

type reader struct {
	data []byte
	pos  int
}

// NewReader creates a new reader instance.
func NewReader(data []byte) io.Reader {
	return &reader{
		data: data,
		pos:  0,
	}
}

func (r *reader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	// If the last byte read is '\n', we act like bufio.Scanner
	if r.data[r.pos-1] == '\n' {
		return n, nil
	}

	return n, nil
}
