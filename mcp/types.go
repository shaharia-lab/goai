package mcp

import (
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"time"
)

// JSON-RPC 2.0 error codes
const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternal       = -32603
)

// ServerConfig holds configuration options for the MCP server
type ServerConfig struct {
	Logger               *logrus.Logger
	Tracer               trace.Tracer
	EnableMetrics        bool
	MaxToolExecutionTime time.Duration
	MaxRequestSize       int
	AllowedOrigins       []string
	EnableStdio          bool
	EnableSSE            bool
	EnableWebSocket      bool
}

// ServerCapabilities defines what features the server supports
type ServerCapabilities struct {
	Resources struct {
		Subscribe   bool `json:"subscribe"`
		ListChanged bool `json:"listChanged"`
	}
	Tools struct {
		ListChanged bool `json:"listChanged"`
		Execute     bool `json:"execute"`
	}
}

// LifecycleState represents the server's current state
type LifecycleState int

const (
	StateUninitialized LifecycleState = iota
	StateInitializing
	StateRunning
	StateShuttingDown
	StateStopped
)
