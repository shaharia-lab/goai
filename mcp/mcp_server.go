package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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
	state               LifecycleState
	version             Version
	shutdown            chan struct{}
	mu                  sync.RWMutex
	initManager         *InitializationManager
	progressMgr         *ProgressManager
	capabilities        ServerCapabilities
	resourceMgr         *ResourceManager
	toolMgr             *ToolManager
	promptMgr           *PromptManager
	connections         map[*Connection]bool
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
		resourceMgr:         NewResourceManager(),
		toolMgr:             NewToolManager(),
		promptMgr:           NewPromptManager(),
		connections:         make(map[*Connection]bool),
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
		Prompts: struct {
			List        bool `json:"list"`
			ListChanged bool `json:"listChanged"`
		}{
			List:        true,
			ListChanged: true,
		},
		Logging: struct {
			SetLevel bool `json:"setLevel"`
		}{SetLevel: true},
		Sampling: struct {
			CreateMessage bool `json:"createMessage"`
		}{CreateMessage: true},
		Roots: struct {
			List        bool `json:"list"`
			ListChanged bool `json:"listChanged"`
		}{
			List:        true,
			ListChanged: true,
		},
		Completion: struct {
			Complete bool `json:"complete"`
		}{Complete: true},
		Progress: struct {
			Report bool `json:"report"`
		}{Report: true},
		Ping: struct {
			Enabled bool `json:"enabled"`
		}{Enabled: true},
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

	s.initManager = NewInitializationManager(s)
	s.progressMgr = NewProgressManager()

	// Register base handlers
	s.handlers.Register("initialize", s.handleInitialize)
	s.handlers.Register("shutdown", s.handleShutdown)
	s.handlers.Register("ping", s.handlePing)

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

func (s *Server) handleMessage(conn *Connection, msg Message) {
	if msg.Method == "" {
		conn.SendMessage(*NewErrorResponse(msg.ID, ErrorCodeInvalidRequest, "Method is required", nil))
		return
	}

	handler, exists := s.handlers.Get(msg.Method)
	if !exists {
		conn.SendMessage(*NewErrorResponse(msg.ID, ErrorCodeMethodNotFound, "Method not found", nil))
		return
	}

	result, err := handler(conn, msg.Params)
	if err != nil {
		if jsonRPCErr, ok := err.(*Error); ok {
			conn.SendMessage(*NewErrorResponse(msg.ID, jsonRPCErr.Code, jsonRPCErr.Message, jsonRPCErr.Data))
		} else {
			conn.SendMessage(*NewErrorResponse(msg.ID, ErrorCodeInternal, err.Error(), nil))
		}
		return
	}

	conn.SendMessage(*NewResponse(msg.ID, result))
}

func (s *Server) handlePing(conn *Connection, params json.RawMessage) (interface{}, error) {
	return map[string]interface{}{
		"timestamp": time.Now().UnixNano() / int64(time.Millisecond),
	}, nil
}

func (s *Server) handleShutdown(conn *Connection, params json.RawMessage) (interface{}, error) {
	// Implement shutdown logic
	return nil, nil
}

func (s *Server) handleLoggingSetLevel(conn *Connection, params json.RawMessage) (interface{}, error) {
	// Implement logging level setting logic
	return nil, nil
}

func (s *Server) handleSamplingCreateMessage(conn *Connection, params json.RawMessage) (interface{}, error) {
	// Implement sampling message creation logic
	return nil, nil
}

func (s *Server) registerDefaultHandlers() {
	s.handlers.Register("initialize", s.handleInitialize)
	s.handlers.Register("ping", s.handlePing)
	s.handlers.Register("resources/list", s.handleResourcesList)
	s.handlers.Register("resources/read", s.handleResourcesRead)
	s.handlers.Register("resources/subscribe", s.handleResourcesSubscribe)
	s.handlers.Register("tools/list", s.handleToolsList)
	s.handlers.Register("tools/call", s.handleToolsCall)
	s.handlers.Register("prompts/list", s.handlePromptsList)
	s.handlers.Register("logging/setLevel", s.handleLoggingSetLevel)
	s.handlers.Register("sampling/createMessage", s.handleSamplingCreateMessage)
	s.handlers.Register("shutdown", s.handleShutdown)
	s.handlers.Register("logging/setLevel", s.handleLoggingSetLevel)
	s.handlers.Register("sampling/createMessage", s.handleSamplingCreateMessage)
}
