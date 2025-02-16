package goai

import (
	"context"
	"fmt"

	"github.com/shaharia-lab/goai/mcp"
)

type ToolsProvider struct {
	mcpClient *mcp.Client
	toolsList []mcp.Tool
}

// NewToolsProvider creates a new ToolsProvider with no initial MCP client or tools.
func NewToolsProvider() *ToolsProvider {
	return &ToolsProvider{
		mcpClient: &mcp.Client{},
		toolsList: make([]mcp.Tool, 0),
	}
}

// AddTools injects tools into the provider. If tools are added, MCP client usage is restricted.
func (p *ToolsProvider) AddTools(tools []mcp.Tool) error {
	if p.mcpClient.IsInitialized() == true {
		return fmt.Errorf("cannot add tools as MCP client is already set")
	}
	p.toolsList = tools
	return nil
}

// AddMCPClient injects the MCP client into the provider. If tools are added, MCP client usage is restricted.
func (p *ToolsProvider) AddMCPClient(client *mcp.Client) error {
	if len(p.toolsList) > 0 {
		return fmt.Errorf("cannot set MCP client as tools are already added")
	}
	p.mcpClient = client
	return nil
}

// ListTools returns the list of tools available in the provider.
func (p *ToolsProvider) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if p.mcpClient.IsInitialized() == false && len(p.toolsList) == 0 {
		return []mcp.Tool{}, nil
	}

	if p.mcpClient.IsInitialized() == true {
		return p.mcpClient.ListTools(ctx)
	}

	return p.toolsList, nil
}

// ExecuteTool executes a tool with the specified ID, name, and parameters.
func (p *ToolsProvider) ExecuteTool(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	if p.mcpClient.IsInitialized() == false && len(p.toolsList) == 0 {
		return mcp.CallToolResult{}, fmt.Errorf("no tools available")
	}

	for _, tool := range p.toolsList {
		if tool.Name == params.Name {
			return tool.Handler(ctx, params)
		}
	}

	if p.mcpClient.IsInitialized() {
		return p.mcpClient.CallTool(ctx, params)
	}

	return mcp.CallToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
}
