# plan: Task 6.12: Standalone Plan Linter Subcommand & MCP Tool

**Status:** Open

Expose the local plan template validation and relative link conformance checks as a standalone CLI subcommand (e.g., `powerword lint-plans`) and package it as an independent MCP tool. This allows external CI/CD pipelines, pre-commit hooks, or coding agents to validate plans interactively without launching a full code review.

## User Review Required

> [!NOTE]
> **Speculative Optimization**:
> Exposing this check as a subcommand gives external pipelines a lightweight, low-footprint way to check plans. Packaging it as an MCP tool also lets parent coordinator agents programmatically double-check plan format conformance when drafting plans.

---

## Proposed Changes

### Command Subsystem

#### [NEW] [lint_plans.go](file://../cmd/powerword/lint_plans.go)
- Create a new Cobra command `lint-plans` under the root CLI.
- Flag support:
  - `--path`: Specify target path to validate (defaults to plans/ directory).
  - `--template`: Path to custom plan template.
- Main logic:
  - Load and resolve the template.
  - Run `ValidatePlans` on the target path.
  - If any errors are found, print them to standard output/error and exit with non-zero code `1`. Otherwise, exit with `0`.

### MCP Plugin Integration

#### [NEW] [server.go](file://../internal/plugins/linter/server.go)
- Define a standard MCP server structure utilizing `github.com/modelcontextprotocol/go-sdk/mcp`.
- Register the `validate_plans` tool:
  - Parameters:
    - `workspace_root` (string): Absolute path to the repository workspace.
    - `plan_template_path` (string): Optional custom template path.
  - Tool handler:
    - Invoke the Go internal `ValidatePlans(workspace_root, cfg)`.
    - If errors are found, return `IsError = true` with a formatted list of all validation errors.

---

## Verification Plan

### Automated Tests
- Create `internal/plugins/linter/server_test.go` to mock MCP calls to `validate_plans` and assert correct JSON responses on valid and invalid plan repositories.
- Command: `go test -v ./internal/plugins/linter/...`

### Manual Verification
- Run `powerword lint-plans` on the local workspace and ensure it succeeds.
- Introduce a plan template error, run `powerword lint-plans` and assert the command exits with exit code 1.
