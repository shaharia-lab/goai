package anth2oai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeOAIRequest(t *testing.T, body string) *oaiRequest {
	t.Helper()
	var r oaiRequest
	require.NoError(t, json.Unmarshal([]byte(body), &r))
	return &r
}

func TestTranslateRequest_Basic(t *testing.T) {
	in := decodeOAIRequest(t, `{
		"model": "glm-4.5-air",
		"messages": [
			{"role": "system", "content": "You are terse."},
			{"role": "developer", "content": "Be exact."},
			{"role": "user", "content": "Reply with exactly: OK"}
		],
		"max_tokens": 64,
		"temperature": 1.5,
		"top_p": 0.9,
		"stop": "STOP"
	}`)

	out, err := translateRequest(in, 4096)
	require.NoError(t, err)

	assert.Equal(t, "glm-4.5-air", out.Model)
	assert.Equal(t, int64(64), out.MaxTokens)
	require.NotNil(t, out.Temperature)
	assert.Equal(t, 1.0, *out.Temperature, "temperature clamped to [0,1]")
	require.NotNil(t, out.TopP)
	assert.Equal(t, 0.9, *out.TopP)
	assert.Equal(t, []string{"STOP"}, out.StopSequences)

	require.Len(t, out.System, 2)
	assert.Equal(t, "You are terse.", out.System[0].Text)
	assert.Equal(t, "Be exact.", out.System[1].Text)

	require.Len(t, out.Messages, 1)
	assert.Equal(t, "user", out.Messages[0].Role)
	require.Len(t, out.Messages[0].Content, 1)
	assert.Equal(t, "text", out.Messages[0].Content[0].Type)
	assert.Equal(t, "Reply with exactly: OK", out.Messages[0].Content[0].Text)
}

func TestTranslateRequest_DefaultMaxTokens(t *testing.T) {
	in := decodeOAIRequest(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	assert.Equal(t, int64(4096), out.MaxTokens)
}

func TestTranslateRequest_MaxCompletionTokensFallback(t *testing.T) {
	in := decodeOAIRequest(t, `{"model":"m","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":128}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	assert.Equal(t, int64(128), out.MaxTokens)
}

func TestTranslateRequest_TemperatureClampLow(t *testing.T) {
	in := decodeOAIRequest(t, `{"model":"m","messages":[{"role":"user","content":"hi"}],"temperature":-0.5}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	require.NotNil(t, out.Temperature)
	assert.Equal(t, 0.0, *out.Temperature)
}

func TestTranslateRequest_StopArray(t *testing.T) {
	in := decodeOAIRequest(t, `{"model":"m","messages":[{"role":"user","content":"hi"}],"stop":["A","B"]}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, out.StopSequences)
}

func TestTranslateRequest_Tools(t *testing.T) {
	in := decodeOAIRequest(t, `{
		"model": "m",
		"messages": [{"role":"user","content":"weather?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}
			}
		}],
		"tool_choice": "required"
	}`)

	out, err := translateRequest(in, 4096)
	require.NoError(t, err)

	require.Len(t, out.Tools, 1)
	assert.Equal(t, "get_weather", out.Tools[0].Name)
	assert.Equal(t, "Get weather", out.Tools[0].Description)
	assert.JSONEq(t, `{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`,
		string(out.Tools[0].InputSchema))

	require.NotNil(t, out.ToolChoice)
	assert.Equal(t, "any", out.ToolChoice.Type)
}

func TestTranslateRequest_ToolChoiceVariants(t *testing.T) {
	cases := []struct {
		name       string
		toolChoice string
		wantType   string
		wantName   string
		dropTools  bool
	}{
		{"auto", `"auto"`, "auto", "", false},
		{"required", `"required"`, "any", "", false},
		{"named", `{"type":"function","function":{"name":"foo"}}`, "tool", "foo", false},
		{"none", `"none"`, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"m","messages":[{"role":"user","content":"x"}],` +
				`"tools":[{"type":"function","function":{"name":"foo","parameters":{"type":"object"}}}],` +
				`"tool_choice":` + tc.toolChoice + `}`
			out, err := translateRequest(decodeOAIRequest(t, body), 4096)
			require.NoError(t, err)
			if tc.dropTools {
				assert.Nil(t, out.Tools, "tools omitted for tool_choice=none")
				assert.Nil(t, out.ToolChoice)
				return
			}
			require.NotNil(t, out.ToolChoice)
			assert.Equal(t, tc.wantType, out.ToolChoice.Type)
			assert.Equal(t, tc.wantName, out.ToolChoice.Name)
		})
	}
}

func TestTranslateRequest_AssistantToolCallsAndToolResult(t *testing.T) {
	in := decodeOAIRequest(t, `{
		"model": "m",
		"messages": [
			{"role":"user","content":"weather in Paris?"},
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Paris\"}"}}
			]},
			{"role":"tool","tool_call_id":"call_1","content":"18C"}
		]
	}`)

	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	require.Len(t, out.Messages, 3)

	// user
	assert.Equal(t, "user", out.Messages[0].Role)

	// assistant tool_use
	assert.Equal(t, "assistant", out.Messages[1].Role)
	require.Len(t, out.Messages[1].Content, 1)
	tu := out.Messages[1].Content[0]
	assert.Equal(t, "tool_use", tu.Type)
	assert.Equal(t, "call_1", tu.ID)
	assert.Equal(t, "get_weather", tu.Name)
	assert.JSONEq(t, `{"location":"Paris"}`, string(tu.Input))

	// tool result becomes a user message with tool_result block
	assert.Equal(t, "user", out.Messages[2].Role)
	require.Len(t, out.Messages[2].Content, 1)
	tr := out.Messages[2].Content[0]
	assert.Equal(t, "tool_result", tr.Type)
	assert.Equal(t, "call_1", tr.ToolUseID)
	assert.JSONEq(t, `"18C"`, string(tr.Content))
}

func TestTranslateRequest_MergesConsecutiveToolResults(t *testing.T) {
	in := decodeOAIRequest(t, `{
		"model":"m",
		"messages":[
			{"role":"tool","tool_call_id":"a","content":"1"},
			{"role":"tool","tool_call_id":"b","content":"2"}
		]
	}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1, "adjacent tool results merge into one user message")
	assert.Len(t, out.Messages[0].Content, 2)
}

func TestTranslateRequest_ImageParts(t *testing.T) {
	in := decodeOAIRequest(t, `{
		"model":"m",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"describe"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}},
			{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}
		]}]
	}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)
	blocks := out.Messages[0].Content
	require.Len(t, blocks, 3)

	assert.Equal(t, "text", blocks[0].Type)

	require.NotNil(t, blocks[1].Source)
	assert.Equal(t, "base64", blocks[1].Source.Type)
	assert.Equal(t, "image/png", blocks[1].Source.MediaType)
	assert.Equal(t, "AAAA", blocks[1].Source.Data)

	require.NotNil(t, blocks[2].Source)
	assert.Equal(t, "url", blocks[2].Source.Type)
	assert.Equal(t, "https://example.com/x.png", blocks[2].Source.URL)
}

func TestTranslateRequest_EmptyToolArgsDefaultsToObject(t *testing.T) {
	in := decodeOAIRequest(t, `{
		"model":"m",
		"messages":[{"role":"assistant","content":null,"tool_calls":[
			{"id":"c","type":"function","function":{"name":"f","arguments":""}}
		]}]
	}`)
	out, err := translateRequest(in, 4096)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)
	assert.JSONEq(t, `{}`, string(out.Messages[0].Content[0].Input))
}

func TestParseDataURI(t *testing.T) {
	mt, data, ok := parseDataURI("data:image/jpeg;base64,Zm9v")
	require.True(t, ok)
	assert.Equal(t, "image/jpeg", mt)
	assert.Equal(t, "Zm9v", data)

	_, _, ok = parseDataURI("data:image/png,notbase64")
	assert.False(t, ok, "non-base64 data URIs are not decoded")
}
