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
	} `json:"resources"`
	Tools struct {
		ListChanged bool `json:"listChanged"`
		Execute     bool `json:"execute"`
	} `json:"tools"`
	Prompts struct {
		List        bool `json:"list"`
		ListChanged bool `json:"listChanged"`
	} `json:"prompts"`
	Logging struct {
		SetLevel bool `json:"setLevel"`
	} `json:"logging"`
	Sampling struct {
		CreateMessage bool `json:"createMessage"`
	} `json:"sampling"`
	Roots struct {
		List        bool `json:"list"`
		ListChanged bool `json:"listChanged"`
	} `json:"roots"`
	Completion struct {
		Complete bool `json:"complete"`
	} `json:"completion"`
	Progress struct {
		Report bool `json:"report"`
	} `json:"progress"`
	Ping struct {
		Enabled bool `json:"enabled"`
	} `json:"ping"`
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

type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
	Capabilities    map[string]interface{} `json:"capabilities"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
