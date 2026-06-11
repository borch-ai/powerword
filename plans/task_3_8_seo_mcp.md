# plan: Task 3.8: Amazon KDP SEO & Metadata Agent Plugin (pw-mcp-seo)

**Status:** Open (Issue #TBD)

This task implements a native Go-based MCP server (`pw-mcp-seo`) that queries market search volume data, extracts competitor details, and suggests high-yield keywords, description copy, and formatting targets for books.

## User Review Required

> [!NOTE]
> This plugin will query Amazon auto-complete search volumes and public HTML scrape endpoints. We must implement proper request rate-limiting and user-agent rotations to prevent IP bans or throttling.

## Proposed Changes

### SEO Plugin Component
Create a new directory `internal/plugins/seo/` to contain the keyword search engines.

#### [NEW] [seo.go](file:///Users/human/code/powerword/internal/plugins/seo/seo.go)
- Implement search endpoint query helpers.
- Implement HTML parsing logic for Amazon product pages.
- Expose the following MCP tools:
  - `seo_analyze_niche`: Queries search suggestion networks to retrieve trending search terms and competitor metadata.
  - `seo_generate_listing`: Generates title, subtitle, seven search keywords, and description copy optimized for Amazon index algorithms.

#### [NEW] [seo_test.go](file:///Users/human/code/powerword/internal/plugins/seo/seo_test.go)
- Unit tests validating search suggestion parsers and mock scraper responses using `httptest.NewServer`.

### CLI Manifest Integration
#### [MODIFY] [internal/config/config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Register the `pw-mcp-seo` server within the native plugin registry under the config key `[plugins.seo]`.

---

## Verification Plan

### Automated Tests
- Run `go test ./internal/plugins/seo/...` to assert parser resilience on varying HTML inputs.
- Enforce the 91% unit test coverage requirement.

### Manual Verification
- Run `powerword "analyze the keyword search volume for existential nursery rhymes and draft listing metadata"` and verify it outputs key suggestions.
