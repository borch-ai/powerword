# plan: Task 3.10: Market Intelligence Plugin (`pw-mcp-trends`)

**Status:** Open (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Implement `pw-mcp-trends`, a native Go MCP server that provides market demand
intelligence by wrapping two data sources:

1. **Amazon Autocomplete** — free, unauthenticated; returns keyword completions
   that proxy real-time search volume.
2. **SerpAPI Google Trends** — paid (~$0.005/call); returns a 3-month interest
   time-series used to compute a trend velocity score.

Primary consumer: the **Kiln `scout` engine** (Task 2.2 / Task 2.3). This plugin
lifts the market intelligence logic out of Kiln's `internal/scout` package and
makes it reusable by any MCP client in the ecosystem (Lamplighter, future agents).

> [!IMPORTANT]
> **Blocks Kiln Task 2.2 (SerpAPI Google Trends Adapter).** Kiln Task 2.2 was
> originally designed with a direct HTTP client as a placeholder. Now that
> `pw-mcp-trends` is promoted to Phase 3, Task 2.2 must be updated to consume
> this MCP server instead of shipping a bespoke SerpAPI client. Kiln Task 2.2
> must not be started until this plan is complete.

---

## User Review Required

> [!NOTE]
> **SerpAPI Key is optional.** If no `POWERWORD_SERP_API_KEY` is set (or the key
> is absent from `powerword.toml`), the `serp` source is silently skipped and
> only Amazon completions are returned. Consumers receive a partial but valid
> result — never an error due to a missing key.
>
> [!NOTE]
> **`TrendSource` is the extension point.** Additional backends (Reddit Trends,
> TikTok For Business) can be added without changing the MCP tool interface.
> Each backend implements `internal/trends.TrendSource`; the router selects
> sources from the `sources` tool parameter.

---

## Proposed Changes

### New Binary

#### [NEW] [main.go](file://../../cmd/pw-mcp-trends/main.go)

Standard MCP server entry point — identical scaffolding to `pw-mcp-seo`:

```go
func main() {
    srv := mcp.NewServer(&mcp.Implementation{
        Name:    "pw-mcp-trends",
        Version: "0.1.0",
    }, nil)
    srv.AddTool(&mcp.Tool{Name: "score_niche",         ...}, handleScoreNiche)
    srv.AddTool(&mcp.Tool{Name: "get_trend_velocity",  ...}, handleTrendVelocity)
    if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

---

### MCP Tools

#### Tool: `score_niche`

Returns ranked keyword candidates for a seed niche term.

**Input schema:**

```json
{
  "keyword": "string",
  "limit":   "integer (default: 10)",
  "sources": ["amazon", "serp"]
}
```

**Output:** Array of `ScoredCandidate`:

```json
[
  { "keyword": "radon detector home", "completions": 8, "trend_score": 0.92 },
  ...
]
```

#### Tool: `get_trend_velocity`

Returns a single keyword's trend velocity derived from the SerpAPI 3-month
time-series.

**Input schema:**

```json
{
  "keyword": "string",
  "period":  "string (default: '3-m')"
}
```

**Output:**

```json
{ "trend_score": 0.87, "direction": "rising" }
```

---

### `TrendSource` Interface & Implementations

#### [NEW] [source.go](file://../../internal/trends/source.go)

```go
// TrendSource is the extension interface for market intelligence backends.
type TrendSource interface {
    // Score returns keyword candidates with demand and velocity signals.
    Score(ctx context.Context, keyword string, limit int) ([]Candidate, error)
    Name() string
}

// Candidate is the unified output type across all backends.
type Candidate struct {
    Keyword     string
    Completions int     // Amazon completion rank (0 if source is serp-only)
    TrendScore  float64 // normalized velocity ratio (0.0–1.0)
}
```

Implementations in the same package:

- **`AmazonAutocomplete`** — `GET https://completion.amazon.com/api/2017/suggestions?...`
  Parses suggestion list; sets `Completions = len(suggestions)`, `TrendScore = 0`
  (velocity is only available from SerpAPI).
- **`SerpAPITrends`** — `GET https://serpapi.com/search?engine=google_trends&...`
  Parses `interest_over_time.timeline_data`; computes:

  ```text
  recentAvg = mean(last 4 weeks)
  olderAvg  = mean(weeks 5–12)
  TrendScore = min(1.0, recentAvg / max(olderAvg, 1))
  ```

  Returns `Completions = 0` (only SerpAPI provides velocity).

The MCP handler merges results by keyword, summing `Completions` and taking the
max `TrendScore` across sources.

#### [NEW] [source_test.go](file://../../internal/trends/source_test.go)

Mock HTTP servers:

- Amazon: correct completion JSON → correct `Candidate` list.
- SerpAPI: fixture time-series → correct `TrendScore` (rising ≥0.9, flat ≈1.0, declining <0.7).
- SerpAPI: empty key → zero-score candidates, no error.
- SerpAPI: non-200 → error propagated.
- Both: `context.Context` cancellation propagates.

---

### Configuration

#### [MODIFY] [powerword.example.toml](file://../../powerword.example.toml)

Add a `[plugins.trends]` section:

```toml
[plugins.trends]
# SerpAPI key for Google Trends velocity scores.
# If unset, only Amazon completion data is returned.
serp_api_key = ""
```

---

## Verification Plan

### Automated Tests

```bash
go test -race ./cmd/pw-mcp-trends/... ./internal/trends/...
make check-coverage   # ≥91%
make lint
make fmt
```

### Manual Verification

1. Build: `go build -o bin/pw-mcp-trends ./cmd/pw-mcp-trends`.
2. Run: `./bin/pw-mcp-trends` — server starts and waits for MCP JSON on stdin.
3. Send a `score_niche` MCP tool call; verify a ranked `ScoredCandidate` array
   is returned.
4. Configure Kiln (`.kiln.toml`) with `mcp_trends_bin = "../powerword/bin/pw-mcp-trends"`.
5. Run `kiln scout --niche "radon detector" --dry-run` — confirms end-to-end.
