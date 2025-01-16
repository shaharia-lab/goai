# AWS Bedrock Provider

You need the necessary credentials to configure the AWS SDK.

## Setup

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)
//.....
// Initialize provider with Bedrock client
ctx := context.Background()

awsConfig, err := config.LoadDefaultConfig(ctx)
if err != nil {
    log.Printf("failed to load AWS config: %w", err)
    return
}

bedrockClient := bedrockruntime.NewFromConfig(awsConfig)

provider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
    Client: bedrockClient,
    Model:  "anthropic.claude-3-5-sonnet-20240620-v1:0",
})
```

## Message Handling

```go
// Configure request
cfg := goai.NewRequestConfig(goai.WithMaxToken(1000))
llm := goai.NewLLMRequest(cfg, provider)

// Generate response
response, err := llm.Generate([]goai.LLMMessage{
    {Role: goai.UserRole, Text: "Explain the quantum theory"},
})

if err != nil {
    panic(err)
}

// Print response
log.Println(response.Text)
fmt.Printf("Tokens: Input=%d, Output=%d\n", response.TotalInputToken, response.TotalOutputToken)
```

## Response Structure

```go
type LLMResponse struct {
    Text             string
    TotalInputToken  int
    TotalOutputToken int
    CompletionTime   float64
}
```
