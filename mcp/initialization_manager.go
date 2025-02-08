// initialization_manager.go
package mcp

type InitializationManager struct {
	server *Server
	state  LifecycleState
}

func NewInitializationManager(server *Server) *InitializationManager {
	return &InitializationManager{
		server: server,
		state:  StateUninitialized,
	}
}

func (im *InitializationManager) Initialize(conn *Connection, params InitializeParams) error {
	if im.state != StateUninitialized {
		return NewMCPError(ErrorCodeInvalidRequest, "Server already initialized", nil)
	}

	im.state = StateInitializing

	// Verify protocol version
	if params.ProtocolVersion != "1.0" {
		im.state = StateUninitialized
		return NewMCPError(ErrorCodeInvalidParams, "Unsupported protocol version", nil)
	}

	// Store client capabilities and info
	// TODO: Implement capability negotiation

	im.state = StateRunning
	return nil
}

func (im *InitializationManager) Shutdown() error {
	if im.state != StateRunning {
		return NewMCPError(ErrorCodeInvalidRequest, "Server not running", nil)
	}

	im.state = StateShuttingDown
	// Perform cleanup tasks
	im.state = StateStopped
	return nil
}
