# Language Models (LLM)

The `goai` package provides a flexible interface for working with various
Large Language Models (LLMs). This module supports multiple providers
and offers features like streaming responses, configurable parameters,
and tool calling capabilities.

## Basic Example

Here's a complete example using OpenAI's GPT-3.5:

<!-- markdownlint-disable -->
```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai"
)

func main() {
	// Create OpenAI LLM Provider
	llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
		Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
		Model:  openai.ChatModelGPT3_5Turbo,
	})

	// Configure LLM Request
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(100),
		goai.WithTemperature(0.7),
	), llmProvider)

	ctx := context.Background()

	// Generate response
	response, err := llm.Generate(ctx, []goai.LLMMessage{
		{Role: goai.UserRole, Text: "Explain quantum computing"},
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("Response: %s\n", response.Text)
	fmt.Printf("Input token: %d, Output token: %d", response.TotalInputToken, response.TotalOutputToken)
}
```
<!-- markdownlint-enable -->

## Tool Calling

The package supports tool calling capabilities, currently available with the Anthropic & OpenAI provider.
This allows the LLM to interact with custom tools during the conversation.

### Implementing Tools

To create a tool, define your tool according to the following definition:

<!-- markdownlint-disable -->
```go
// import 
// Tool
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     func(ctx context.Context, params CallToolParams) (CallToolResult, error)
}
```

```go
import 

tools := []Tool{
		{
			Name:        "get_weather",
			Description: "Get weather for location",
			InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "location": {"type": "string"}
                },
                "required": ["location"]
            }`),
			Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
				var input struct {
					Location string `json:"location"`
				}
				json.Unmarshal(params.Arguments, &input)
				return CallToolResult{
					Content: []ToolResultContent{{
						Type: "text",
						Text: fmt.Sprintf("Weather in %s: Sunny", input.Location),
					}},
				}, nil
			},
		},
	}

toolsProvider := goai.NewToolsProvider()
err := toolsProvider.AddTools(tools)
```
<!-- markdownlint-enable -->

If you want to integrate MCP (Model Context Protocol) server with your tool, you can use the MCP Client.
Details about how to use MCP Client can be found [here](md#client)

```go
//lient := 
err := toolsProvider.AddMCPClient(lient)
```

**Note: If you provide both tools and MCP client, the attached tools will be used for tool calling.**

### Using Tools with LLM Provider

<!-- markdownlint-disable -->
```go
func main() {
	llmConfig := goai.NewRequestConfig(
        goai.WithMaxToken(100),
        goai.WithTemperature(0.7),
        goai.UseToolsProvider(toolsProvider),   // Use tools provider
    )

    llm := goai.NewLLMRequest(llmConfig, llmProvider)
    // rest of the code
    // ....
}
```
<!-- markdownlint-enable -->

## Streaming Example

For streaming responses:

<!-- markdownlint-disable -->
```go
    // Generate streaming response
    stream, err := llm.GenerateStream(context.Background(), []goai.LLMMessage{
        {Role: goai.UserRole, Text: "Explain quantum computing"},
    })

    if err != nil {
        panic(err)
    }

    for resp := range stream {
        if resp.Error != nil {
            fmt.Printf("Error: %v\n", resp.Error)
            break
        }
        if resp.Done {
            break
        }
        fmt.Print(resp.Text)
    }
```
<!-- markdownlint-enable -->

## Message Types

```go
type LLMMessageRole string

const (
    UserRole      LLMMessageRole = "user"
    AssistantRole LLMMessageRole = "assistant"
    SystemRole    LLMMessageRole = "system"
)

type LLMMessage struct {
    Role LLMMessageRole
    Text string
}
```

## Configuration Options

```go
config := goai.NewRequestConfig(
    goai.WithMaxToken(500),                 // Maximum token length. Default: 500
    goai.WithTopP(0.5),                     // Top-p sampling. Default: 0.5
    goai.WithTemperature(0.5),              // Sampling temperature. Default: 0.5
    goai.WithTopK(40),                      // Top-k sampling. Default: 50 
    goai.UseToolsProvider(toolsProvider),  // Use tools provider
)
```

## Available Providers

### Google Gemini

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ✅ Tool calling (synchronous + streaming)

For using OpenAI compatible API, please refer to the [OpenAI Compatible](#gemini-from-google) section. _(Not recommended)_

```go
geminiAPIKey := os.Getenv("GEMINI_API_KEY")
geminiModel := "gemini-2.0-flash"

googleGeminiService, err := goai.NewGoogleGeminiService(geminiAPIKey, geminiModel)
if err != nil {
    log.Fatalf("Error creating Real Gemini Service: %v", err)
}

llmProvider, err := goai.NewGeminiProvider(googleGeminiService, observability.NewDefaultLogger())
if err != nil {
    panic(fmt.Sprintf("Error creating Gemini provider: %v", err))
}
```

For more details about Google Gemini API can be found [here](https://ai.google.dev/gemini-api/docs)

### OpenAI

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ❌ Tool calling

```go
llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
    Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
    Model:  openai.ChatModelGPT3_5Turbo,
})
```

For more details about OpenAI API can be found [here](https://platform.openai.com/docs/api-reference/chat)

### Anthropic

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ✅ Tool calling (synchronous + streaming)

```go
llmProvider := goai.NewAnthropicLLMProvider(goai.AnthropicProviderConfig{
    Client: goai.NewAnthropicClient(os.Getenv("ANTHROPIC_API_KEY")),
    Model:  anthropic.ModelClaude3_5Sonnet20241022,
})
```

For more details about Anthropic API can be found [here](https://docs.anthropic.com/en/api/getting-started)

### AWS Bedrock

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ✅ Tool calling (synchronous + streaming)

```go
// Initialize AWS config
awsConfig, err := config.LoadDefaultConfig(ctx)
if err != nil {
    log.Printf("failed to load AWS config: %w", err)
    return
}

llmProvider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
    Client: bedrockruntime.NewFromConfig(awsConfig),
    Model:  "anthropic.claude-3-5-sonnet-20240620-v1:0",
}, goai.NewDefaultLogger())
```

For more details about AWS Bedrock API can be found [here](https://docs.aws.amazon.com/bedrock/latest/userguide/getting-started-api.html)

### OpenAI Compatible

#### Gemini from Google

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ❌ Tool calling

For full support, it's recommended to use the dedicated Google Gemini provider as shown [above](#google-gemini).

```go
// Create client
client := goai.NewOpenAIClient(
    "{GEMINI_API_KEY}",
    option.WithBaseURL("https://generativelanguage.googleapis.com/v1beta/openai/"),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "gemini-2.0-flash",
})
```

For more details about Google Gemini API can be found [here](https://ai.google.dev/gemini-api/docs/openai)

#### xAI

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ❌ Tool calling

```go
// Create client
client := goai.NewOpenAIClient(
    "{XAI_API_KEY}",
    option.WithBaseURL("https://api.x.ai/v1"),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "grok-2-latest",
})
```

For more details about xAI API can be found [here](https://docs.x.ai/docs/guides/chat)

#### Hugging Face

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ❌ Tool calling

```go
// Create client
client := goai.NewOpenAIClient(
    "{HUGGING_FACE_API_KEY}",
    option.WithBaseURL("https://api-inference.huggingface.co/v1/"),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "microsoft/phi-4",
})
```

For more details about Hugging Face API can be found [here](https://huggingface.co/docs/api-inference/en/getting-started)

#### DeepSeek

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ❌ Tool calling

```go
// Create client
client := goai.NewOpenAIClient(
    "{DEEP_SEEK_API_KEY}",
    option.WithBaseURL("https://api.deepseek.com/v1/"),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "deepseek-chat",
})
```

For more details about DeepSeek API can be found [here](https://api-docs.deepseek.com/)

#### Mistral AI

**Supported features:**

- ✅ Streaming responses
- ✅ Non-streaming (synchronous) responses
- ❌ Tool calling

```go
// Create client
client := goai.NewOpenAIClient(
    "{MISTRAL_AI_API_KEY}",
    option.WithBaseURL("https://api.mistral.ai/v1/"),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "mistral-small-latest",
})
```

For more details about Mistral AI API can be found [here](https://docs.mistral.ai/api/)

## Response Extraction

The package provides several extractors to parse structured data from LLM responses:

### JSON Extraction

```go
// Define a struct for your data
type Person struct {
    Name    string `json:"name"`
    Age     int    `json:"age"`
    IsAdmin bool   `json:"is_admin"`
}

// Create response with JSON data
response := LLMResponse{
    Text: `{"name": "John Doe", "age": 30, "is_admin": true}`,
}

// Create extractor and extract data
var person Person
extractor := goai.NewJSONExtractor(&person)
result, err := extractor.Extract(response)
if err != nil {
    panic(err)
}

// Use the extracted data
extracted := result.(*Person)
fmt.Printf("Name: %s, Age: %d\n", extracted.Name, extracted.Age)
```

### XML Extraction

```go
// Define a struct for your data
type Product struct {
    Name  string  `xml:"name"`
    Price float64 `xml:"price"`
}

// Create response with XML data
response := LLMResponse{
    Text: `<product><name>Widget</name><price>29.99</price></product>`,
}

// Create extractor and extract data
var product Product
extractor := goai.NewXMLExtractor(&product)
result, err := extractor.Extract(response)
if err != nil {
    panic(err)
}
```

### Custom Tag Extraction

```go
// Create response with tagged content
response := LLMResponse{
    Text: "The SQL query is: <query>SELECT * FROM users WHERE age > 21</query>",
}

// Extract content between custom tags
extractor := goai.NewTagExtractor("query")
result, err := extractor.Extract(response)
if err != nil {
    panic(err)
}

query := result.(string)
fmt.Println("Extracted query:", query)
```

### Table Extraction

```go
// Create response with markdown table
response := LLMResponse{
    Text: `| Name  | Score |
|--------|--------|
| Alice  | 95     |
| Bob    | 87     |`,
}

// Extract table data
extractor := goai.NewTableExtractor("markdown")
result, err := extractor.Extract(response)
if err != nil {
    panic(err)
}

// Access table data
table := result.(*goai.Table)
fmt.Printf("Headers: %v\n", table.Headers)
for _, row := range table.Rows {
    fmt.Printf("Row: %v\n", row)
}
```
