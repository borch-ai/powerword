# plan: Task 5.7: Multi-Language Coverage Plugin (LCOV & Cobertura)

**Status:** Open

Extend Powerword's coverage checking capabilities to support SvelteKit/TypeScript (Vitest LCOV format) and Python (Cobertura XML format). This will be implemented as a standalone MCP server `pw-mcp-coverage` that can parse multi-language coverage reports, enabling automated quality checks for repositories like Knurl.

## User Review Required

> [!IMPORTANT]
> **MCP Tool Interface**:
> The `pw-mcp-coverage` server will expose the tool `check_coverage` with arguments:
> - `threshold` (number): The minimum coverage percentage required (e.g., `85.0`).
> - `profile_path` (string): The path to the coverage profile file (e.g., `app/coverage/lcov.info`).
> - `format` (string, optional): `"lcov"`, `"cobertura"`, `"go"`, or `"auto"` (auto-detected based on extension/header).

---

## Proposed Changes

### MCP Coverage Plugin

#### [NEW] [main.go](file:///Users/human/code/powerword/scripts/lint_markdown/main.go)
- Create the standard CLI wrapper to initialize and run the stdio-based MCP server.

#### [NEW] [server.go](file:///Users/human/code/powerword/internal/mcp/critic/server.go)
- Implement the MCP server utilizing the `modelcontextprotocol/go-sdk`.
- Register the `check_coverage` tool.
- Implement parsing drivers for:
  - **LCOV (`lcov.info`):** Read line-by-line matching `SF:` (Source File), `DA:` (Line Execution count), `LF:` (Lines Found), and `LH:` (Lines Hit) to aggregate block coverage.
  - **Cobertura (`coverage.xml`):** Parse XML data extracting line rates and statement hits for packages, classes, and individual lines.
  - **Go (`coverage.out`):** Delegate or natively parse Go statements.
- If coverage falls below the threshold, return a tool error alongside the summary.

### Powerword CLI Integration

#### [MODIFY] [check_coverage.go](file:///Users/human/code/powerword/cmd/powerword/check_coverage.go)
- Update the subcommand to auto-detect file formats (e.g., if target file is `lcov.info` or XML).
- If format is non-Go, instantiate and call the `pw-mcp-coverage` tool via standard MCP process spawning.

#### [MODIFY] [Makefile](file:///Users/human/code/powerword/Makefile)
- Add the `cmd/pw-mcp-coverage/main.go` target to the compilation list to build `bin/pw-mcp-coverage` as a compiled plugin.

---

## Verification Plan

### Automated Tests
- Create unit tests in `internal/mcp/coverage/server_test.go` verifying:
  - Parsing a sample SvelteKit/Vitest `lcov.info` file correctly calculates covered lines.
  - Parsing a sample pytest `coverage.xml` file correctly calculates covered lines.
  - Correct threshold enforcement and fail/pass exits.
- Command: `go test -v ./internal/mcp/coverage/...`
- Ensure test coverage on the new package meets the **91% threshold**.

### Manual Verification
- In the `Knurl` repository (a SvelteKit codebase in `/Users/human/code/Knurl` running Vitest):
  1. Generate the coverage file: run `npm run test:unit -- --coverage` (outputting `lcov.info`).
  2. Run `powerword check-coverage 90.0 app/coverage/lcov.info`.
  3. Verify it successfully aggregates TypeScript coverage and returns the correct status.
