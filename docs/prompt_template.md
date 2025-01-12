# Prompt Templates

The prompt template functionality allows you to create dynamic prompts using Go's template system. This makes it easy to create reusable prompt patterns with variable substitution.

## Basic Usage

```go
import "github.com/shaharia-lab/goai"

// Create template
template := &ai.LLMPromptTemplate{
    Template: "Hello {{.Name}}! Tell me about {{.Topic}}.",
    Data: map[string]interface{}{
        "Name":  "Alice",
        "Topic": "artificial intelligence",
    },
}

// Parse template
prompt, err := template.Parse()
if err != nil {
    log.Fatal(err)
}

// Use with LLM
response, err := request.Generate([]ai.LLMMessage{
    {Role: ai.UserRole, Text: prompt},
})
```

## Template Syntax

Supports standard Go template features:
- Variables: `{{.VarName}}`
- Conditionals: `{{if .Condition}} ... {{end}}`
- Loops: `{{range .Items}} ... {{end}}`
- Functions: `{{.Value | function}}`

## Common Patterns

### System Prompts
```go
template := &ai.LLMPromptTemplate{
    Template: `You are an AI assistant specialized in {{.Field}}.
Your task is to {{.Task}}.
Please use {{.Style}} language.`,
    Data: map[string]interface{}{
        "Field": "mathematics",
        "Task":  "explain complex concepts",
        "Style": "simple",
    },
}
```

### Structured Queries
```go
template := &ai.LLMPromptTemplate{
    Template: `Given the following {{.DataType}}:
{{range .Examples}}
- {{.}}
{{end}}
Please {{.Task}} and provide {{.OutputFormat}} output.`,
    Data: map[string]interface{}{
        "DataType": "customer feedback",
        "Examples": []string{"Good service", "Fast delivery"},
        "Task": "analyze sentiment",
        "OutputFormat": "JSON",
    },
}
```

## Best Practices

1. **Error Handling**
```go
prompt, err := template.Parse()
if err != nil {
    // Handle parsing error
    return err
}
```

2. **Data Validation**
```go
func createPrompt(name, topic string) (string, error) {
    if name == "" || topic == "" {
        return "", fmt.Errorf("name and topic are required")
    }
    
    template := &ai.LLMPromptTemplate{
        Template: `Hello {{.Name | html}}!
Please explain {{.Topic | html}} in simple terms.`,
        Data: map[string]interface{}{
            "Name":  name,
            "Topic": topic,
        },
    }
    
    return template.Parse()
}
```

3. **HTML Escaping**
```go
// Use html function for content that might contain HTML
Template: `<query>{{.UserInput | html}}</query>`
```

4. **Reusable Templates**
```go
// Define template library
var templates = map[string]*ai.LLMPromptTemplate{
    "explain": {
        Template: `Explain {{.Topic}} in {{.Style}} terms.`,
    },
    "analyze": {
        Template: `Analyze {{.Text}} for {{.Aspect}}.`,
    },
}

// Use templates
func getPrompt(templateName string, data map[string]interface{}) (string, error) {
    tmpl, exists := templates[templateName]
    if !exists {
        return "", fmt.Errorf("template not found: %s", templateName)
    }
    tmpl.Data = data
    return tmpl.Parse()
}
```

## Structure Reference

```go
type LLMPromptTemplate struct {
    // Template string using Go template syntax
    Template string

    // Data for template variables
    Data map[string]interface{}
}
```

## Error Cases

- Template syntax errors
- Missing required variables
- Invalid data types
- Template execution errors

Always handle these errors appropriately in your application.