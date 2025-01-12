# Prompt Templates

## Structure
```go
type LLMPromptTemplate struct {
    Template string
    Data    map[string]interface{}
}
```

## Basic Usage
```go
template := &ai.LLMPromptTemplate{
    Template: "Hello {{.Name}}! Tell me about {{.Topic}}.",
    Data: map[string]interface{}{
        "Name":  "User",
        "Topic": "AI",
    },
}

promptText, err := template.Parse()
if err != nil {
    return err
}
```

## Template Syntax
- Variables: `{{.VarName}}`
- Text blocks: `{{ block "name" . }}...{{ end }}`
- HTML escaping: `{{.UserInput | html}}`

## Error Handling
```go
promptText, err := template.Parse()
if err != nil {
    // Handle template parsing error
    return err
}

messages := []ai.LLMMessage{
    {Role: ai.UserRole, Text: promptText},
}
```