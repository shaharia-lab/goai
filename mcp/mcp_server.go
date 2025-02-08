package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

// Server represents an MCP server instance
type Server struct {
	config              ServerConfig
	logger              *logrus.Logger
	tracer              trace.Tracer
	handlers            *RegisteredHandlers
	cancellationManager *CancellationManager
	resourceManager     *ResourceManager
	transports          []Transport
	capabilities        ServerCapabilities
	state               LifecycleState
	version             Version
	shutdown            chan struct{}
	mu                  sync.RWMutex
}

// NewServer creates a new MCP server instance
func NewServer(config ServerConfig) (*Server, error) {
	// Validate config
	if config.MaxToolExecutionTime <= 0 {
		return nil, fmt.Errorf("invalid MaxToolExecutionTime: must be positive")
	}
	if config.MaxRequestSize <= 0 {
		return nil, fmt.Errorf("invalid MaxRequestSize: must be positive")
	}

	if config.Logger == nil {
		config.Logger = logrus.New()
	}

	s := &Server{
		config:              config,
		logger:              config.Logger,
		tracer:              config.Tracer,
		handlers:            NewRegisteredHandlers(),
		cancellationManager: NewCancellationManager(),
		resourceManager:     NewResourceManager(),
		transports:          make([]Transport, 0),
		shutdown:            make(chan struct{}),
		state:               StateUninitialized,
	}

	// Initialize capabilities and version structure
	s.capabilities = ServerCapabilities{
		Resources: struct {
			Subscribe   bool `json:"subscribe"`
			ListChanged bool `json:"listChanged"`
		}{
			Subscribe:   true,
			ListChanged: true,
		},
		Tools: struct {
			ListChanged bool `json:"listChanged"`
			Execute     bool `json:"execute"`
		}{
			ListChanged: true,
			Execute:     true,
		},
	}

	// Initialize enabled transports
	if config.EnableWebSocket {
		wsTransport := NewWebSocketTransport(config.AllowedOrigins)
		s.transports = append(s.transports, wsTransport)
	}

	if config.EnableStdio {
		stdioTransport := NewStdIOTransport()
		s.transports = append(s.transports, stdioTransport)
	}

	/*if config.EnableSSE {
		sseTransport := NewSSETransport()
		s.transports = append(s.transports, sseTransport)
	}*/

	if len(s.transports) == 0 {
		return nil, fmt.Errorf("no transport enabled, at least one transport must be enabled")
	}

	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	s.state = StateInitializing
	s.mu.Unlock()

	// Start all transports
	for _, t := range s.transports {
		if err := t.Start(); err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.state = StateRunning
	s.mu.Unlock()

	<-ctx.Done()
	return s.Stop()
}

func (s *Server) Stop() error {
	s.mu.Lock()
	s.state = StateShuttingDown
	s.mu.Unlock()

	close(s.shutdown)

	// Stop all transports
	for _, t := range s.transports {
		if err := t.Stop(); err != nil {
			s.logger.WithError(err).Error("Failed to stop transport")
		}
	}

	s.mu.Lock()
	s.state = StateStopped
	s.mu.Unlock()

	return nil
}
