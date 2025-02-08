// server_handlers.go
package mcp

import "encoding/json"

func (s *Server) registerCapabilityHandlers() {
	// Resource handlers
	s.handlers.Register("resources/list", s.handleResourcesList)
	s.handlers.Register("resources/read", s.handleResourcesRead)
	s.handlers.Register("resources/subscribe", s.handleResourcesSubscribe)

	// Tool handlers
	s.handlers.Register("tools/list", s.handleToolsList)
	s.handlers.Register("tools/call", s.handleToolsCall)

	// Prompt handlers
	s.handlers.Register("prompts/list", s.handlePromptsList)
	s.handlers.Register("prompts/get", s.handlePromptsGet)
}

func (s *Server) handleResourcesList(conn *Connection, params json.RawMessage) (interface{}, error) {
	resources := s.resourceMgr.ListResources()
	return resources, nil
}

func (s *Server) handleResourcesRead(conn *Connection, params json.RawMessage) (interface{}, error) {
	var reqParams struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &reqParams); err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, "Invalid parameters", nil)
	}

	resource, err := s.resourceMgr.GetResource(reqParams.URI)
	if err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, err.Error(), nil)
	}

	return resource, nil
}

func (s *Server) handleResourcesSubscribe(conn *Connection, params json.RawMessage) (interface{}, error) {
	var reqParams struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &reqParams); err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, "Invalid parameters", nil)
	}

	s.resourceMgr.Subscribe(reqParams.URI, conn)
	return true, nil
}

func (s *Server) handleToolsList(conn *Connection, params json.RawMessage) (interface{}, error) {
	tools := s.toolMgr.ListTools()
	return tools, nil
}

func (s *Server) handleToolsCall(conn *Connection, params json.RawMessage) (interface{}, error) {
	var reqParams struct {
		ID   string                 `json:"id"`
		Args map[string]interface{} `json:"args"`
	}
	if err := json.Unmarshal(params, &reqParams); err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, "Invalid parameters", nil)
	}

	result, err := s.toolMgr.ExecuteTool(reqParams.ID, reqParams.Args)
	if err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, err.Error(), nil)
	}

	return result, nil
}

func (s *Server) handlePromptsList(conn *Connection, params json.RawMessage) (interface{}, error) {
	prompts := s.promptMgr.ListPrompts()
	return prompts, nil
}

func (s *Server) handlePromptsGet(conn *Connection, params json.RawMessage) (interface{}, error) {
	var reqParams struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &reqParams); err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, "Invalid parameters", nil)
	}

	prompt, err := s.promptMgr.GetPrompt(reqParams.ID)
	if err != nil {
		return nil, NewErrorResponse(nil, ErrorCodeInvalidParams, err.Error(), nil)
	}

	return prompt, nil
}
