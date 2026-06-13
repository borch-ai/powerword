# plan: Task 6.12: Standalone Linter Suite (CLI, `pw-mcp-linter`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-13
**Unit Test Coverage:** 91.10%

Promote the plan validation and markdown formatting linter logic to a public, reusable package `pkg/linter`. Extend the primary CLI with subcommands `powerword lint-plans` and `powerword lint-go` for CI/CD pipelines and manual developer usage, and package the logic into a new standalone MCP server binary `pw-mcp-linter` that outputs structured JSON errors for autonomous agent loops.

## User Review Required

> [!IMPORTANT]
> **Consolidation**:
> This task combines the standalone linter subcommand (originally Task 6.12) and the Plan Linter MCP Server (originally Task 6.20) into a single, cohesive delivery phase.

---

## Proposed Changes

### Linter Promotion to Public Package

#### [DELETE] [linter.go](file://../../internal/linter/linter.go)
#### [DELETE] [linter_test.go](file://../../internal/linter/linter_test.go)

#### [NEW] [linter.go](file://../../pkg/linter/linter.go)
- Relocate markdown linting logic from `internal/linter/linter.go` to public package `pkg/linter`.

#### [NEW] [validator.go](file://../../pkg/linter/validator.go)
- Move plan template validation and absolute/relative link conformance logic from `internal/review/validator.go` to public package `pkg/linter`.
- Clean up imports to use `github.com/borch-ai/powerword/pkg/config`.

#### [MODIFY] [critic.go](file://../../internal/review/critic.go)
- Update imports to use `github.com/borch-ai/powerword/pkg/linter` for `ValidatePlans` and `FixAbsolutePathsInPlans`.

#### [NEW] [linter_test.go](file://../../pkg/linter/linter_test.go)
#### [NEW] [validator_test.go](file://../../pkg/linter/validator_test.go)
- Migrate respective tests to `pkg/linter`.

---

### Command Subsystem

#### [NEW] [lint.go](file://../../cmd/powerword/lint.go)
- Implement `powerword lint-plans` and `powerword lint-go` commands.
- **`powerword lint-plans`**:
  - Arguments/Flags:
    - `--path`: Directory containing plans (defaults to `./plans`).
    - `--template`: Custom `TEMPLATE.md` path.
  - Logic: Loads resolved template, runs `linter.ValidatePlans`, prints failures to `stderr`, and exits with `1` on error or `0` on success.
- **`powerword lint-go`**:
  - Flags:
    - `--config`: Path to custom `.golangci.yml` (optional).
  - Embed the canonical `.golangci.yml` from root via `//go:embed`.
  - Logic:
    1. Read `go.mod` in the current directory to extract module path prefix (e.g., `github.com/borch-ai/kiln`).
    2. Dynamically replace or set `goimports.local-prefixes` in the embedded config to match the module prefix.
    3. Write the configuration to a temporary file.
    4. Run `golangci-lint run --config <temp_file>` via subprocess execution. If a local `.golangci.yml` is present in the workspace, use it as an override instead.
    5. If `golangci-lint` is missing on the host, fall back to running `go vet` and printing a recommendation to install `golangci-lint`.

---

### Standalone Plugin Entrypoint

#### [NEW] [main.go](file://../../cmd/pw-mcp-linter/main.go)
- Create CLI scaffolding to bootstrap the `pw-mcp-linter` plugin.
- Support reading `POWERWORD_WORKSPACE_ROOT` environment variable or falling back to local working directory.
- Initialize and serve the MCP server over standard I/O (`stdio`).

### MCP Server Tool Registration

#### [NEW] [server.go](file://../../internal/mcp/linter/server.go)
- Define a standard MCP server structure utilizing `github.com/modelcontextprotocol/go-sdk/mcp`.
- Register the `lint_plans` tool with the following JSON schema parameters:
  - `workspace_root` (string, required): Absolute or relative path to the repository workspace containing `plans/`.
  - `plan_template_path` (string, optional): Path to a custom plan template.
- Implement the tool handler:
  1. Call `pkg/linter.ValidatePlans(workspace_root, cfg)`.
  2. If errors are found, return `IsError = false` (as the tool itself executed successfully) but provide a structured JSON payload detailing the failures:
     ```json
     {
       "valid": false,
       "errors": [
         {
           "file": "plans/phase_6/task_6_12_standalone_plan_linter.md",
           "line": 42,
           "message": "path \"/Users/human/code/powerword/pkg/linter\" must be relative, not absolute",
           "rule": "relative_links"
         }
       ]
     }
     ```
  3. If all plans conform, return:
     ```json
     {
       "valid": true,
       "errors": []
     }
     ```

### Build and Makefile Integration

#### [MODIFY] [Makefile](file://../../Makefile)
- Add `pw-mcp-linter` to the `build` target to compile `bin/pw-mcp-linter` if `cmd/pw-mcp-linter` exists.
- Add `pw-mcp-linter` to the `install` target.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./pkg/linter/...` to verify both markdown linting and plan template validation pass.
- Create `internal/mcp/linter/server_test.go` to test:
  - Tool execution with a valid plan workspace returning `{"valid": true}`.
  - Tool execution with validation failures returning `{"valid": false}` and the correct list of structured errors.
- Run tests: `go test -v ./internal/mcp/linter/...`
- Verify `powerword check-coverage` still meets the 91% threshold.

### Manual Verification
- Compile powerword: `make build`.
- Run `./bin/powerword lint-plans` on the local codebase. Verify it exits with `0`.
- Temporarily corrupt a plan file or link label. Run `./bin/powerword lint-plans` and verify it displays the formatting errors and exits with `1`.
- Run `./bin/powerword lint-go` and verify it correctly detects `github.com/borch-ai/powerword` and runs the linter successfully.
- Register `pw-mcp-linter` locally in a test MCP configuration.
- Call the `lint_plans` tool using a mock client (or via an active agent) and assert the JSON output schema conforms to requirements.
