# plan: Task 5.4: MCP Telemetry Migration

**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-07-21
**Unit Test Coverage:** 91.0%

Extract the local Go telemetry pricing databases and calculation code from `powerword`'s shared library into a dedicated standalone repository/package, and refactor `powerword` to consume it as an external stdio MCP server.

## User Review Required

> [!NOTE]
> **Centralized Capability Architecture**:
> To satisfy the ecosystem-wide guidelines, `pw-mcp-telemetry` is implemented directly under `cmd/pw-mcp-telemetry` and `internal/mcp/telemetry` within this repository rather than being placed in a separate repository. This ensures Powerword remains the centralized source of truth for all Borch-AI shared capabilities.

> [!NOTE]
> **Lamplighter Telemetry Compatibility**:
> The JSON-RPC tool returns from `pw-mcp-telemetry` (specifically token counts and estimated costs) conform to Lamplighter's signaling payload requirements (`input_tokens`, `output_tokens`, `estimated_cost`, `daily_quota_cap`). This allows Powerword (or wrapping IDE extensions) to cleanly serialize and publish these metrics directly to the Firebase Realtime Database.

---

## Proposed Changes

### Standalone Server Development

#### [NEW] [server.go](file://../../internal/mcp/telemetry/server.go)
- Implements standard Model Context Protocol Go SDK integration.
- Relocates default pricing database maps and longest prefix matching logic.
- Exposes two tools:
  - `calculate_tokens_cost`
  - `get_model_pricing`

#### [NEW] [main.go](file://../../cmd/pw-mcp-telemetry/main.go)
- Serves as the stdio entry point for the standalone telemetry capability server.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-telemetry/main_integration_test.go)
- End-to-end integration test (gated by `//go:build integration` tag) that builds the plugin binary, launches it as a stdio subprocess via the process manager, and tests the `calculate_tokens_cost` and `get_model_pricing` tool execution handshakes.

### Powerword Client Refactoring

#### [MODIFY] [loop.go](file://../../internal/loop/loop.go)
- Implements `getCostAndUsage` helper to dynamically check the MCP registry for a mounted `telemetry` client.
- If registered, queries the telemetry server via the `telemetry__calculate_tokens_cost` tool.
- If not registered (or on server failure), transparently falls back to local calculations using native structs, ensuring offline capability.
- Updates `checkBudget`, `submitTelemetry`, and `handleSessionSaveAndOutput` to utilize the new helper.

#### [MODIFY] [Makefile](file://../../Makefile)
- Registers and compiles `pw-mcp-telemetry` under standard build targets.

---

## Verification Plan

### Automated Tests
- Telemetry server unit tests: `go test -v ./internal/mcp/telemetry/...` (Passed)
- Subprocess integration test: `go test -v -tags=integration ./cmd/pw-mcp-telemetry/...` (Passed)
- Loop integration tests: `go test -v ./internal/loop/...` (Passed)
- Coverage check: `make check-coverage` (Passed, maintaining 91.0% overall coverage threshold)

### Manual Verification
- Registered `servers.telemetry` in `powerword.toml`.
- Ran execution loop: `./bin/powerword "hello" -m gemini-2.5-flash --accept-all`.
- Verified correct cost calculation and output formatting from the server.
