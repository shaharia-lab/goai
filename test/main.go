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

	resources := []mcp.Resource{
		{
			URI:         "file:///home/shaharia/Projects/goai/bash.sh",
			Name:        "Sample Document",
			Description: "A sample text document for testing",
			MimeType:    "text/plain",
			TextContent: "This is the content of the sample document.",
		},
	}

	prompt := mcp.Prompt{
		Name:        "code_review",
		Description: "Performs a detailed code review analyzing code quality, potential issues, and suggesting improvements",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "language",
				Description: "Programming language of the code (e.g., Go, Python, JavaScript)",
				Required:    true,
			},
			{
				Name:        "code",
				Description: "Source code to be reviewed",
				Required:    true,
			},
			{
				Name:        "focus_areas",
				Description: "Specific areas to focus on (e.g., performance, security, readability)",
				Required:    false,
			},
		},
		Messages: []mcp.PromptMessage{
			{
				Role: "assistant",
				Content: mcp.PromptContent{
					Type: "text",
					Text: "You are an experienced code reviewer. Analyze the provided code and give constructive feedback focusing on:" +
						"\n- Code quality and best practices" +
						"\n- Potential bugs or issues" +
						"\n- Performance considerations" +
						"\n- Security implications" +
						"\n- Readability and maintainability",
				},
			},
			{
				Role: "user",
				Content: mcp.PromptContent{
					Type: "text",
					Text: "Please provide a comprehensive code review with actionable suggestions for improvement.",
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

	server := mcp.NewStdIOServer(
		baseServer,
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
