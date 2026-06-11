# plan: Task 5.4: MCP Telemetry Migration

**Status:** Open (Issue #[TBD])

Extract the local Go telemetry pricing databases and calculation code from `powerword`'s shared library into a dedicated standalone repository `github.com/borch-ai/mcp-telemetry`, and refactor `powerword` to consume it as an external stdio MCP server.

## User Review Required

> [!NOTE]
> **Standalone Repository Creation**:
> This task involves setting up a brand new Git repository `github.com/borch-ai/mcp-telemetry` to build the `pw-mcp-telemetry` binary.

---

## Proposed Changes

### Standalone Server Development

#### [NEW] [mcp-telemetry Repo](file:///Users/human/code/mcp-telemetry)
- Set up a new Go codebase compiling to the binary executable `pw-mcp-telemetry`.
- Implement standard Model Context Protocol Go SDK integration.
- Relocate pricing database arrays and prefix-matching logic from `powerword`'s package.
- Expose the following JSON-RPC tools:
  * `calculate_tokens_cost`
  * `get_model_pricing`

### Powerword Client Refactoring

#### [MODIFY] [config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Remove the local `Pricing` map structure from the global configuration struct.
- Add configuration settings to register and mount `pw-mcp-telemetry` under standard plugins.

#### [MODIFY] [telemetry.go](file:///Users/human/code/powerword/pkg/telemetry/telemetry.go)
- Refactor the cost calculation methods to query the mounted `pw-mcp-telemetry` MCP client connection instead of executing native pricing lookups.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./pkg/telemetry/... ./internal/...`
- Verify that tests correctly spin up a mock telemetry MCP server to test the cost query loops.
