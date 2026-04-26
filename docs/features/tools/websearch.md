# Web Search Tool (`web_search`)

## Summary
- Tool name: `web_search`
- Type: client-side read-only tool
- Source package: `internal/tools/websearch`
- Runtime path: `query.Engine` tool loop -> `tool.Registry.Lookup("web_search")` -> DDG Lite HTTP search

## Input Schema
```json
{
  "type": "object",
  "required": ["query"],
  "additionalProperties": false,
  "properties": {
    "query": { "type": "string", "minLength": 2 },
    "allowed_domains": { "type": "array", "items": { "type": "string" } },
    "blocked_domains": { "type": "array", "items": { "type": "string" } }
  }
}
```

Validation rules:
- `query` must be at least 2 characters.
- `allowed_domains` and `blocked_domains` cannot both be set in one call.

## Output Schema
```json
{
  "query": "string",
  "results": [
    { "title": "string", "url": "string", "snippet": "string" }
  ],
  "durationSeconds": 0.0
}
```

## DDG Lite Client Design
- Endpoint: `https://html.duckduckgo.com/html/?q=<query>`
- Method: `GET`
- Timeout: 15 seconds
- Max results: 10
- Headers: browser-like `User-Agent`, `Accept`, `Accept-Language`

Parser details:
- Uses `golang.org/x/net/html` to parse DOM.
- Extracts each result block from `result`-class containers.
- Reads:
  - title/link from `a.result__a` (with fallback classes)
  - snippet from `result__snippet` / `result-snippet` / `snippet`
- Normalizes DDG redirect URLs (`uddg` query parameter).
- Detects and surfaces anti-bot challenge pages (`anomaly-modal`) as explicit errors.

Filtering:
- Domain allow/block filtering runs after parse.
- Matching supports exact host and subdomain suffix (for example `docs.python.org` matches rule `python.org`).

## Provider Integration
- The tool is registered as a normal client tool in `cmd/argus/main.go`.
- Anthropic special-case `web_search_*` tool schema mapping was removed.
- All providers now use the same `tool_use -> web_search -> tool_result` loop through `query.Engine`.
