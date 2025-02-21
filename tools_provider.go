package goai

import (
	"context"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"time"

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
	ctx, span := observability.StartSpan(ctx, "ToolsProvider.ExecuteTool")
	span.SetAttributes(
		attribute.String("tool_name", params.Name),
		attribute.String("arguments", string(params.Arguments)),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	startTime := time.Now()

	// Record initial state
	if p.mcpClient.IsInitialized() == false && len(p.toolsList) == 0 {
		err = fmt.Errorf("no tools available")
		return mcp.CallToolResult{}, err
	}

	// Try local tools first
	span.AddEvent("CheckingLocalTools",
		trace.WithAttributes(attribute.Int("local_tools_count", len(p.toolsList))))

	for i, tool := range p.toolsList {
		if tool.Name == params.Name {
			span.AddEvent("FoundLocalTool",
				trace.WithAttributes(attribute.Int("tool_index", i)))

			execCtx, execSpan := observability.StartSpan(ctx, "ExecuteTool.Handler")
			execSpan.SetAttributes(
				attribute.String("tool_name", tool.Name),
			)

			result, execErr := tool.Handler(execCtx, params)

			if execErr != nil {
				execSpan.RecordError(execErr)
				execSpan.SetStatus(codes.Error, execErr.Error())
			} else {
				execSpan.SetAttributes(
					attribute.Bool("is_error", result.IsError),
					attribute.Int("content_length", len(result.Content)),
				)
			}
			execSpan.End()

			// Capture execution time in parent span
			span.SetAttributes(
				attribute.Float64("execution_time_ms", float64(time.Since(startTime).Milliseconds())),
				attribute.Bool("is_local_tool", true),
			)

			return result, execErr
		}
	}

	// Try remote MCP client if no local tool found
	if p.mcpClient.IsInitialized() {
		span.AddEvent("FallingBackToMCPClient")

		// Create child span for MCP client call
		mcpCtx, mcpSpan := observability.StartSpan(ctx, "ToolsProvider.ExecuteTool_MCPClient")
		mcpSpan.SetAttributes(
			attribute.String("tool_name", params.Name),
		)

		result, mcpErr := p.mcpClient.CallTool(mcpCtx, params)

		if mcpErr != nil {
			mcpSpan.RecordError(mcpErr)
			mcpSpan.SetStatus(codes.Error, mcpErr.Error())
		} else {
			mcpSpan.SetAttributes(
				attribute.Bool("is_error", result.IsError),
				attribute.Int("content_length", len(result.Content)),
			)
		}
		mcpSpan.End()

		// Capture execution time in parent span
		span.SetAttributes(
			attribute.Float64("execution_time_ms", float64(time.Since(startTime).Milliseconds())),
			attribute.Bool("is_mcp_tool", true),
		)

		return result, mcpErr
	}

	// No tool found
	err = fmt.Errorf("tool not found: %s", params.Name)
	span.SetAttributes(
		attribute.Float64("execution_time_ms", float64(time.Since(startTime).Milliseconds())),
		attribute.Bool("tool_found", false),
	)

	return mcp.CallToolResult{}, err
}
