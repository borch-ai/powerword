# plan: Task 3.8: Amazon KDP SEO & Metadata Agent Plugin (pw-mcp-seo)

**Status:** Complete (Implemented under Go 1.26.4)

This task implements a native Go-based MCP server (`pw-mcp-seo`) that queries market search volume data, extracts competitor details, and suggests high-yield keywords, description copy, and formatting targets for books.

## User Review Required

> [!NOTE]
> This plugin queries public search suggestion endpoints and competitor metadata. It strictly adheres to rate limits, implements standard backoff and caching policies, and uses standard request headers to ensure polite and compliant access.
> - An isolated local cache is created under `~/.cache/powerword/seo-cache` (using safe SHA-256 keys) to store responses and respect rate throttling.
> - Go 1.26.4 is utilized as the development environment.

## Proposed Changes

### SEO Plugin Component
Created new directory `internal/plugins/seo/` containing the keyword search engines.

#### [NEW] [seo.go](file:///Users/human/code/powerword/internal/plugins/seo/seo.go)
- [x] Implement search endpoint query helpers.
- [x] Implement HTML parsing logic for Amazon product pages.
- [x] Expose the following MCP tools:
  - `seo_analyze_niche`: Queries search suggestion networks to retrieve trending search terms and competitor metadata.
  - `seo_generate_listing`: Generates title, subtitle, seven search keywords, and description copy optimized for Amazon index algorithms.

#### [NEW] [seo_test.go](file:///Users/human/code/powerword/internal/plugins/seo/seo_test.go)
- [x] Unit tests validating search suggestion parsers and mock scraper responses using standard mock round trippers and isolated test cache directories.

### CLI Manifest Integration
#### [MODIFY] [config.go](file:///Users/human/code/powerword/pkg/config/config.go)
- [x] Register the SEO plugin configuration structure (cache TTL / rate limit) under the config key `[plugins.seo]`.

---

## Verification Plan

### Automated Tests
- [x] Run `go test ./internal/plugins/seo/...` to assert parser resilience on varying HTML inputs.
- [x] Enforce the 91% unit test coverage requirement (total coverage achieved: **91.40%**).
- [x] Verify code format via `make fmt` and linter checks with `make lint`.

### Manual Verification
- [x] Build binary: `make build`.
- [x] Register `[servers.seo]` in `powerword.toml` to connect CLI to the plugin.
- [x] Run `powerword` agent loops requesting SEO listing generations and verify appropriate outputs.
