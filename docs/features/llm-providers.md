# LLM Providers

## Scope

Argus now separates provider discovery endpoints from runtime model entries.

- A `server` is a discovery target stored in `~/.argus/models.json`.
- A `model` is an executable catalog entry used by the REPL and query engine.
- Imported models keep provider/base URL/API env/context metadata on the model itself so runtime switching stays simple.

## Supported Server Types

### OpenAI-compatible

Used for OpenAI itself and any `/v1`-style compatible endpoint such as local Ollama, vLLM, LM Studio, company gateways, DeepSeek-compatible endpoints, or Qwen-compatible gateways.

Stored fields:
- `alias`
- `display`
- `provider=openai-compat`
- `base_url`
- optional `api_key_env`

Init input:
- `server alias`
- `server url`
- optional `api key env`

URL handling:
- if the user enters `host:port`, Argus automatically normalizes it to `http://host:port`
- if the input URL has no path, Argus probes these candidates in order:
  - `<input>/v1`
  - `<input>`
- if the input URL already has a path, Argus uses that path as the candidate base URL
- the first candidate whose `GET {baseURL}/models` succeeds becomes the stored `base_url`

Discovery:
- `GET {baseURL}/models`
- `Authorization: Bearer <key>` only when `api_key_env` is configured and present in the environment

Runtime:
- `POST {baseURL}/chat/completions`
- no-auth local servers are supported

### Anthropic

Stored fields:
- `alias`
- `display`
- `provider=anthropic`
- `base_url`
- `api_key_env`

Discovery:
- `GET {baseURL}/models?limit=1000`
- headers: `x-api-key`, `anthropic-version: 2023-06-01`
- pagination uses `after_id`

Imported metadata:
- `display_name`
- `max_input_tokens` -> default `ContextWin`

Runtime:
- `POST {baseURL}/messages`

### Gemini

Stored fields:
- `alias`
- `display`
- `provider=gemini`
- `base_url`
- `api_key_env`

Discovery:
- `GET {baseURL}/models?pageSize=1000&key=...`
- filters to models whose `supportedGenerationMethods` include `generateContent`
- pagination uses `nextPageToken`

Imported metadata:
- `displayName`
- `inputTokenLimit` -> default `ContextWin`
- `baseModelId` preferred over `name`

Runtime:
- `POST {baseURL}/models/{model}:streamGenerateContent?alt=sse&key=...`

## Init Flow

`argus --init` performs the following:

1. Ensures `~/.argus/settings.json`, `~/.argus/models.json`, and history file exist.
2. Opens an interactive menu.
3. Lets the user register multiple servers.
4. Discovers models from a selected server.
5. Lets the user import multiple models with explicit per-model context windows.
6. For discovered models, alias defaults to sanitized `model id` and is only re-asked on collisions.
7. Persists the catalog back to `~/.argus/models.json`.

## Context Window Enforcement

Argus now uses the stored `ContextWin` before dispatching a request.

- If estimated prompt tokens already exceed the model context window, the turn is rejected locally.
- If `promptTokens + MaxTokens` exceeds the window, `MaxTokens` is clamped and a REPL notice is emitted.

## References

- OpenAI models list: https://developers.openai.com/api/reference/resources/models/methods/list
- Anthropic models list: https://platform.claude.com/docs/en/api/models/list
- Gemini models list: https://ai.google.dev/api/models#v1beta.models.list
