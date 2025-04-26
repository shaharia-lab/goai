package goai

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ToolsProvider struct {
	lient     *Client
	toolsList []Tool
}

// NewToolsProvider creates a new ToolsProvider with no initial MCP client or tools.
func NewToolsProvider() *ToolsProvider {
	return &ToolsProvider{
		lient:     &Client{},
		toolsList: make([]Tool, 0),
	}
}

// AddTools injects tools into the provider. If tools are added, MCP client usage is restricted.
func (p *ToolsProvider) AddTools(tools []Tool) error {
	if p.lient.IsInitialized() == true {
		return fmt.Errorf("cannot add tools as MCP client is already set")
	}
	p.toolsList = tools
	return nil
}

// AddMCPClient injects the MCP client into the provider. If tools are added, MCP client usage is restricted.
func (p *ToolsProvider) AddMCPClient(client *Client) error {
	if len(p.toolsList) > 0 {
		return fmt.Errorf("cannot set MCP client as tools are already added")
	}
	p.lient = client
	return nil
}

// ListTools returns the list of tools available in the provider.
func (p *ToolsProvider) ListTools(ctx context.Context, allowedTools []string) ([]Tool, error) {
	var tools []Tool
	var err error

	if p.lient.IsInitialized() {
		tools, err = p.lient.ListTools(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		tools = p.toolsList
	}

	if len(allowedTools) > 0 {
		allowedToolsMap := make(map[string]bool)
		for _, tool := range allowedTools {
			allowedToolsMap[tool] = true
		}

		filteredTools := make([]Tool, 0)
		for _, tool := range tools {
			if allowedToolsMap[tool.Name] {
				filteredTools = append(filteredTools, tool)
			}
		}
		return filteredTools, nil
	}

	return tools, nil
}

// ExecuteTool executes a tool with the specified ID, name, and parameters.
func (p *ToolsProvider) ExecuteTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	ctx, span := StartSpan(ctx, "ToolsProvider.ExecuteTool")
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
	if p.lient.IsInitialized() == false && len(p.toolsList) == 0 {
		err = fmt.Errorf("no tools available")
		return CallToolResult{}, err
	}

	// Try local tools first
	span.AddEvent("CheckingLocalTools",
		trace.WithAttributes(attribute.Int("local_tools_count", len(p.toolsList))))

	for i, tool := range p.toolsList {
		if tool.Name == params.Name {
			span.AddEvent("FoundLocalTool",
				trace.WithAttributes(attribute.Int("tool_index", i)))

			execCtx, execSpan := StartSpan(ctx, "ExecuteTool.Handler")
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
	if p.lient.IsInitialized() {
		span.AddEvent("FallingBackToMCPClient")

		// Create child span for MCP client call
		tx, pan := StartSpan(ctx, "ToolsProvider.ExecuteTool_MCPClient")
		pan.SetAttributes(
			attribute.String("tool_name", params.Name),
		)

		result, rr := p.lient.CallTool(tx, params)

		if rr != nil {
			pan.RecordError(rr)
			pan.SetStatus(codes.Error, rr.Error())
		} else {
			pan.SetAttributes(
				attribute.Bool("is_error", result.IsError),
				attribute.Int("content_length", len(result.Content)),
			)
		}
		pan.End()

		// Capture execution time in parent span
		span.SetAttributes(
			attribute.Float64("execution_time_ms", float64(time.Since(startTime).Milliseconds())),
			attribute.Bool("is_tool", true),
		)

		return result, rr
	}

	// No tool found
	err = fmt.Errorf("tool not found: %s", params.Name)
	span.SetAttributes(
		attribute.Float64("execution_time_ms", float64(time.Since(startTime).Milliseconds())),
		attribute.Bool("tool_found", false),
	)

	return CallToolResult{}, err
}
