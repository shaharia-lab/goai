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

	resources := []mcp.Resource{
		{
			URI:         "file:///tmp/hello_world.txt",
			Name:        "Hello World",
			Description: "A sample text document for testing",
			MimeType:    "text/plain",
			TextContent: "This is the content of the sample document.",
		},
	}

	prompt := mcp.Prompt{
		Name:        "code_review",
		Description: "Performs a detailed code review and suggesting improvements",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "code",
				Description: "Source code to be reviewed",
				Required:    true,
			},
		},
		Messages: []mcp.PromptMessage{
			{
				Role: "user",
				Content: mcp.PromptContent{
					Type: "text",
					Text: "Please review this code: {{code}}",
				},
			},
		},
	}

	resourceManager, _ := mcp.NewResourceManager(resources)
	toolManager, _ := mcp.NewToolManager([]mcp.ToolHandler{weatherTool})
	promptManager, _ := mcp.NewPromptManager([]mcp.Prompt{prompt})

	baseServer, err := mcp.NewBaseServer(
		mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
		mcp.UseTools(toolManager),
		mcp.UseResources(resourceManager),
		mcp.UsePrompts(promptManager),
	)
	if err != nil {
		panic(err)
	}

	server := mcp.NewSSEServer(baseServer)
	server.SetAddress(":8080")

	/*server := mcp.NewStdIOServer(
		baseServer,
		os.Stdin,
		os.Stdout,
	)*/
	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
