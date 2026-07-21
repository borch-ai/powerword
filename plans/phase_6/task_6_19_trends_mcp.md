# plan: Task 6.19: Market Intelligence Plugin (`pw-mcp-trends`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-15
**Unit Test Coverage:** 89.9% (internal/plugins/trends), 64.1% (cmd/pw-mcp-trends), project-wide overall 91.2%

Implement `pw-mcp-trends`, a native Go MCP server that provides market demand intelligence by wrapping Amazon Autocomplete (free, unauthenticated) and SerpAPI Google Trends. Primary consumer: the Kiln `scout` engine. This plugin lifts the market intelligence logic out of Kiln's `internal/scout` package and makes it reusable by any MCP client.

## User Review Required

> [!IMPORTANT]
> **This task does NOT replace Kiln's Phase 2 implementation.** Kiln Phase 2 (Tasks 2.1–2.5) implements the adapters directly as library code for speed-to-value. This plugin is the Phase 6 target that Kiln will eventually migrate to, making the intelligence layer reusable by any tool in the ecosystem (including Lamplighter and future agents). Build this after Kiln Phase 2 is working and validated.
>
> [!NOTE]
> **Defines the `TrendSource` interface.** The plugin must be designed so additional backends (Reddit Trends API, TikTok For Business API) can be added without changing the MCP tool interface. Backends implement a Go interface; the MCP server calls them via a router.

## Proposed Changes

### New Binary: `cmd/pw-mcp-trends/`

#### [NEW] [main.go](file://../../cmd/pw-mcp-trends/main.go)

Standard MCP server entry point:

```go
func main() {
    srv := mcp.NewServer(&mcp.Implementation{
        Name:    "pw-mcp-trends",
        Version: "0.1.0",
    }, nil)
    srv.AddTool(&mcp.Tool{Name: "score_niche", ...}, handleScoreNiche)
    srv.AddTool(&mcp.Tool{Name: "get_trend_velocity", ...}, handleTrendVelocity)
    if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

### MCP Tools

#### Tool: `score_niche`

**Input schema:**

```json
{
    "keyword": "string",
    "limit": "integer (default: 10)",
    "sources": ["amazon", "serp"]
}
```

**Output:** Array of `ScoredCandidate` objects with `keyword`, `anxiety_score`, `trend_score`, `completions`.

#### Tool: `get_trend_velocity`

**Input schema:**

```json
{
    "keyword": "string",
    "period": "string (default: '3-m')"
}
```

**Output:** `{ "trend_score": 0.87, "direction": "rising" }`

### `TrendSource` Interface

#### [NEW] [source.go](file://../../internal/plugins/trends/source.go)

```go
type TrendSource interface {
    Score(ctx context.Context, keyword string, limit int) ([]Candidate, error)
    Name() string
}
```

Implementations: `AmazonAutocomplete`, `SerpAPITrends`.

Mirror and then supersede the equivalent code in `kiln/internal/scout/`.

### Tests

#### [NEW] [source_test.go](file://../../internal/plugins/trends/source_test.go)

- Mock HTTP servers for Amazon and SerpAPI.
- MCP tool handler tests with mock sources.
- 91%+ coverage.

---

## Verification Plan

### Automated Tests

- `go test -race ./cmd/pw-mcp-trends/... ./internal/plugins/trends/...`
- `make check-coverage` — ≥91%

### Manual Verification

1. `./bin/pw-mcp-trends` — starts and waits for MCP input on stdin.
2. Send a `score_niche` tool call via MCP JSON; verify response contains ranked candidates.
3. Test against Kiln: configure `powerword.mcp_trends_bin` in `.kiln.toml` and migrate Kiln scout to use the MCP tool.
