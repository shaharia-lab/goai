package mcp

import (
	"context"
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
func NewServer(config ServerConfig) *Server {
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
		shutdown:            make(chan struct{}),
		state:               StateUninitialized,
	}

	s.registerDefaultHandlers()
	s.initializeCapabilities()

	return s
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
