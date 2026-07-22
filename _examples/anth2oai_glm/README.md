# anth2oai — GLM/Z.ai example

Talk to an **Anthropic-format** endpoint (GLM/Z.ai, Kimi, DeepSeek, …) using the
**OpenAI SDK**, via the [`anth2oai`](../../anth2oai) package.

## Run

```bash
export ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
export ANTHROPIC_AUTH_TOKEN=<your token>
go run ./_examples/anth2oai_glm
```

Expected output (content depends on the model):

```
non-streaming: "OK" (tokens: 23)
streaming:     one, two, three, four, five
```

## What it shows

- `anth2oai.New(base, token)` returns a normal `*openai.Client`.
- `client.Chat.Completions.New(...)` — non-streaming, with a system prompt.
- `client.Chat.Completions.NewStreaming(...)` — token-by-token streaming.

Nothing is hardcoded: the base URL and token come only from the environment. The
upstream model id (`glm-4.5-air`) is passed through verbatim — there is no model
aliasing.
