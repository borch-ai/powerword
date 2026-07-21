# plan: Task 6.11: Generalized MCP Critic Server

**Status:** Completed
**Unit Test Coverage:** 91%
**Go Version:** 1.26.4
**Date Completed:** 2026-06-11

Refactor the existing Powerword Local Critic subsystem into a standalone, generalized Model Context Protocol (MCP) server (`pw-mcp-critic`). This will allow the critic to be used as a standardized `review_workspace` tool across any Golang project (including Pithos), eliminating the need to duplicate prompt engineering, diff extraction, and LLM orchestration logic.

## User Review Required

> [!IMPORTANT]
> **MCP Tool Interface**:
> The proposed `pw-mcp-critic` server will expose a single primary tool: `review_workspace`.
> It will accept `plan_content` (string) and `validation_command` (string). Are there any additional parameters you need for generalized cross-project reviews?
>
> [!WARNING]
> **Refactoring Impact**:
> We will update Powerword's own internal `review` command to consume `pw-mcp-critic` instead of running the logic directly, ensuring we "dogfood" the generalized MCP server.

---

## Proposed Changes

### MCP Server Entrypoint

#### [NEW] [main.go](file://../../cmd/pw-mcp-critic/main.go)

- Create the standard CLI scaffolding to initialize and serve the MCP protocol over `stdio`.
- Initialize `pkg/config` and `pkg/llm` specifically for the critic provider.

### MCP Server Implementation

#### [NEW] [server.go](file://../../internal/mcp/critic/server.go)

- Implement the MCP server utilizing `github.com/modelcontextprotocol/go-sdk/mcp`.
- Register the `review_workspace` tool with the following JSON schema parameters:
  - `plan_content`: The markdown text of the implementation plan (passed by the orchestrator).
  - `validation_command`: The command to execute locally (e.g., `make all` or `make lint build check-coverage`).
- The handler will:
  1. Execute the `validation_command` and capture `stdout`/`stderr`.
  2. If the validation command fails, return a verdict of `REJECT` immediately to the client without calling the LLM to save tokens and fail fast.
  3. Extract the git diff (using `pw-mcp-git` or local `git` commands).
  4. Construct the strict reviewer prompt using the provided `plan_content`, validation logs, and diff.
  5. Query the LLM engine and return the textual analysis containing `VERDICT: ACCEPT` or `VERDICT: REJECT`.
  6. The verdict parser trims whitespace, backticks, asterisks, and quotes to ensure robust classification.

### Dogfooding the Plugin

#### [MODIFY] [critic.go](file://../../internal/review/critic.go)

- Refactor `VerifyWorkspace` to launch `pw-mcp-critic` via `internal/mcp.NewServerProcess`.
- Call the `review_workspace` tool via the MCP client, passing the loaded GitHub Issue content and `"make all"`.
- This removes the hardcoded LLM prompt logic from the CLI and delegates it entirely to the MCP server.

### Build and Makefile Updates

#### [MODIFY] [Makefile](file://../../Makefile)

- Add `cmd/pw-mcp-critic/main.go` to the `build` target so `bin/pw-mcp-critic` is compiled alongside other plugins.

---

## Verification Plan

### Automated Tests

- `go test ./internal/mcp/critic/...` (unit tests for server and tools)
- `go test ./cmd/pw-mcp-critic/...` (verifying entrypoint configuration loading and stdio execution)
- `make check-coverage` (validating unit test coverage across both `internal/...` and `pkg/...` to ensure it meets the strict 91% threshold - final result achieved: 91.10%)

### Manual Verification

- Invoked `./bin/powerword review --local` to verify the dogfooding critic client correctly runs validation (`make all`), diff extraction, and LLM analysis via the newly refactored standalone MCP critic server.
- Verified that validation command compile/lint failures correctly trigger instant fail-fast rejections.
