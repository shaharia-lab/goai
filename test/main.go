package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"os"
)

/*func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	flag.Parse()

	server := mcp.NewSSEServer(mcp.NewServerBuilder(
		mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
	))
	server.SetAddress(*addr)

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}*/

func main() {

	weatherTool := NewWeatherTool("get_weather", "Get the current weather for a given location.", json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`))

	toolManager := mcp.NewToolManager([]mcp.ToolHandler{weatherTool})

	server := mcp.NewStdIOServer(
		mcp.NewServerBuilder(
			mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
			mcp.UseToolManager(toolManager),
		),
		os.Stdin,
		os.Stdout,
	)
	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}

type WeatherTool struct {
	name        string
	description string
	inputSchema json.RawMessage
}

func NewWeatherTool(name, description string, inputSchema json.RawMessage) *WeatherTool {
	return &WeatherTool{
		name:        name,
		description: description,
		inputSchema: inputSchema,
	}
}

func (wt *WeatherTool) GetName() string {
	return wt.name
}

func (wt *WeatherTool) GetDescription() string {
	return wt.description
}

func (wt *WeatherTool) GetInputSchema() json.RawMessage {
	return wt.inputSchema
}

func (wt *WeatherTool) Handler(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	// Parse input
	var input struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		return mcp.CallToolResult{}, err
	}

	// Return result
	return mcp.CallToolResult{
		Content: []mcp.ToolResultContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Weather in %s: Sunny, 72°F", input.Location),
			},
		},
	}, nil
}
