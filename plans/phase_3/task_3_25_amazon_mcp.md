# plan: Task 3.25: Amazon Search MCP Plugin (`pw-mcp-amazon`)

**Status:** Completed
**Go Version:** 1.26.5
**Date Completed:** 2026-07-14
**Unit Test Coverage:** 96.20% (overall 91.10%)

## Goal Description

Build a native Go Model Context Protocol (MCP) server `pw-mcp-amazon` exposing a tool to query the commercial Amazon Search API (via Rainforest or ScaleSerp) to retrieve product listing counts for keywords.

## User Review Required

> [!IMPORTANT]
> **API Credentials and Mock fallback**:
> By default, the plugin requires `api_key` to be configured. If `api_key` is explicitly set to `"mock"`, the plugin runs in mock mode returning `4200` to allow local dry-runs and offline integration testing without requiring a live Rainforest/ScaleSerp paid subscription. If `api_key` is empty, it returns a configuration error.

---

## Proposed Changes

### Configuration Layer

#### [MODIFY] [config.go](file://../../pkg/config/config.go)
- Add `AmazonConfig` struct containing `APIKey` and `BaseURL` fields.
- Add `Amazon` field of type `AmazonConfig` to `PluginsConfig` struct.
- In `setDefaults`, register defaults for `plugins.amazon.api_key` (empty) and `plugins.amazon.base_url` (empty).
- In `bindPluginEnvVars`, bind the environment variables `POWERWORD_AMAZON_API_KEY`, `AMAZON_API_KEY`, and `POWERWORD_AMAZON_BASE_URL`.

#### [MODIFY] [powerword.example.toml](file://../../powerword.example.toml)
- Document the Amazon plugin settings block:
  ```toml
  [servers.amazon]
     command = "go"
     args = ["run", "./cmd/pw-mcp-amazon"]

  [plugins.amazon]
  api_key = ""
  ```

#### [MODIFY] [powerword.toml](file://../../powerword.toml) (workspace-local, uncommitted)
- Register the compiled binary server path (this local configuration change is workspace-local and not committed/included in the Pull Request):
  ```toml
  [servers.amazon]
     command = "./bin/pw-mcp-amazon"
  ```

### Amazon Plugin Implementation

#### [NEW] [amazon.go](file://../../internal/plugins/amazon/amazon.go)
- Core service that manages calls to the ScaleSerp/Rainforest APIs.
- Implements `GetListingCount(ctx context.Context, keyword string) (int, error)`.
- Respects context cancellation and propagates errors.

#### [NEW] [amazon_test.go](file://../../internal/plugins/amazon/amazon_test.go)
- Unit tests for the `AmazonService`. Includes tests for mock fallbacks and simulated HTTP responses.

### MCP Server Entry Point

#### [NEW] [main.go](file://../../cmd/pw-mcp-amazon/main.go)
- Entry point for the `pw-mcp-amazon` binary.
- Sets up an MCP server session using `modelcontextprotocol/go-sdk`.
- Registers the tool:
  - `get_amazon_listing_count`: Returns total listing count for a search keyword on Amazon.

#### [NEW] [main_test.go](file://../../cmd/pw-mcp-amazon/main_test.go)
- Stdio transport tests for the MCP server to verify tool call invocation, parameter parsing, and error flows.

#### [NEW] [main_bootstrap_test.go](file://../../cmd/pw-mcp-amazon/main_bootstrap_test.go)
- Bootstrap test for configuration parsing and server setup, verifying the initialization pipeline runs smoothly.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-amazon/main_integration_test.go)
- Integration test running the built binary and verifying correct tools enumeration and end-to-end execution.

### Build and Roadmap Integration

#### [MODIFY] [Makefile](file://../../Makefile)
- Define `AMAZON_PLUGIN=pw-mcp-amazon`.
- Add compilation commands under the `build` and `install` targets to include the Amazon plugin.

#### [MODIFY] [ROADMAP.md](file://../../ROADMAP.md)
- Add **Task 3.25: Amazon Search MCP Plugin (`pw-mcp-amazon`)** to Phase 3.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/amazon/... ./cmd/pw-mcp-amazon/...` to verify correctness.
- Run `make check-coverage` to ensure unit test coverage meets the project-wide 91% threshold.
- Run `make lint` to verify clean AST styling and `gosec` compliance.

### Manual Verification
- Compile the plugin: `make build`.
- Run the compiled binary manually:
  ```bash
  ./bin/pw-mcp-amazon
  ```
  Send JSON-RPC stdio payload to verify it processes the tool correctly.
