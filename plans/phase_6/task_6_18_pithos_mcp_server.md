# plan: Task 6.18: Speculative — Pithos Pipeline MCP Server

**Status:** Completed
**Unit Test Coverage:** 93.3%
**Go Version:** 1.26.5
**Date Completed:** 2026-07-08

Wrap the Pithos book production pipeline behind a formal MCP server (`pw-mcp-pithos`). This enables Kiln and Lamplighter to invoke and monitor `initiate`, `brew`, `assemble`, and `deploy` stages via standard MCP protocol instead of raw subprocess calls. This is the long-term upgrade path for Kiln's forge integration (Kiln Phase 4 → Phase 6 migration).

## User Review Required

> [!IMPORTANT]
> **Subprocess Invocation vs. Library Import**:
> Since Powerword is the infrastructure layer and shouldn't depend on Pithos code directly, `pw-mcp-pithos` will wrap calls to the `pithos` CLI binary using `os/exec`. This means the `pithos` binary must be available in the `PATH` of the environment where this MCP server runs. Does this align with the expected deployment model?

> [!WARNING]
> **Progress Monitoring**:
> `pithos` stages can take a long time (especially `brew`). How should the MCP server handle long-running operations? Should it block and stream stdout/stderr back as progress notifications, or return a job ID for polling? Standard MCP tools usually block until completion. We plan to have the tool block and capture stdout/stderr, returning the final output.

## Open Questions

1. Do we need tools for each individual stage (`pithos_initiate`, `pithos_brew`, etc.), or a single `pithos_run` tool that takes a `stage` argument? I've proposed discrete tools for better schema validation by LLMs.
2. Should `pithos_deploy` take additional arguments like `target` (e.g., `kdp`, `epub`) or does it rely entirely on the project's configuration?

## Proposed Changes

### MCP Server Entrypoint

#### [NEW] [main.go](file://../../cmd/pw-mcp-pithos/main.go)
- Create the standard CLI scaffolding for `pw-mcp-pithos` to initialize and serve the MCP protocol over `stdio`.
- Wire up the server using `github.com/modelcontextprotocol/go-sdk/mcp`.

### Pipeline Execution Logic

#### [NEW] [server.go](file://../../internal/mcp/pithos/server.go)
- Implement the MCP server tool definitions and handlers.
- Register discrete tools for the pipeline stages:
  - `pithos_initiate`: Takes `project_path` and `theme` arguments.
  - `pithos_brew`: Takes `project_path` argument.
  - `pithos_assemble`: Takes `project_path` argument.
  - `pithos_deploy`: Takes `project_path` argument.
- The handler will:
  1. Validate the `project_path` exists.
  2. Use `os/exec` to execute the corresponding `pithos <stage> --dir <project_path>` command.
  3. Capture stdout and stderr streams.
  4. Return the combined output as the tool result, or an error if the command fails (returning the stderr logs for the LLM to analyze).

### Build and Makefile Updates

#### [MODIFY] [Makefile](file://../../Makefile)
- Add `cmd/pw-mcp-pithos/main.go` to the `build` target so `bin/pw-mcp-pithos` is compiled alongside other plugins.

---

## Verification Plan

### Automated Tests
- `go test ./internal/mcp/pithos/...` (unit tests mocking `os/exec` using a fake test binary to simulate Pithos output).
- `go test ./cmd/pw-mcp-pithos/...` (verifying entrypoint configuration loading and stdio execution).

### Manual Verification
- Compile `make build`.
- Use a test MCP client (or `powerword` chat if available) to connect to `./bin/pw-mcp-pithos`.
- Invoke the `pithos_initiate` tool on a dummy path and verify it successfully calls the installed `pithos` binary.
