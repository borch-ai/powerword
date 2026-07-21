# plan: Task 6.28: Advanced Integration & Subprocess Testing Suite Expansion

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-20
**Unit Test Coverage:** 91.1%

Expand the integration test suite to include subprocess stdio tests for the remaining MCP servers (`pw-mcp-critic`, `pw-mcp-linter`, `pw-mcp-kdp-math`, `pw-mcp-seo`) and compiled CLI integration tests for `powerword review` and `powerword link-issue` subcommands using mock stub dependencies.

## User Review Required

> [!NOTE]
> **Build Tag Separation**:
> Like Task 6.15, these new tests will utilize the `//go:build integration` build tag. This ensures they are excluded from fast unit test cycles and are only executed via `make test-integration`.

---

## Proposed Changes

### MCP Subprocess Integration Suite

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-critic/main_integration_test.go)

- Create an integration test with the `//go:build integration` tag for the local critic MCP server.
- Compile `pw-mcp-critic` dynamically in `TestMain`.
- Spawn `pw-mcp-critic` as a stdio subprocess, construct a mock LLM service (or direct to a test endpoint), and test the `review_workspace` tool.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-linter/main_integration_test.go)

- Create an integration test with the `//go:build integration` tag for the plan validator MCP server.
- Compile `pw-mcp-linter` dynamically in `TestMain`.
- Spawn `pw-mcp-linter` as a stdio subprocess, initialize a temporary plans directory, and run the `lint_plans` tool to verify JSON result formatting.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-kdp-math/main_integration_test.go)

- Create an integration test with the `//go:build integration` tag.
- Compile `pw-mcp-kdp-math` dynamically in `TestMain`.
- Spawn the plugin subprocess and verify stdio connectivity and successful execution of math tools (`kdp_calculate_geometry`, etc.).

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-seo/main_integration_test.go)

- Create an integration test with the `//go:build integration` tag.
- Compile `pw-mcp-seo` dynamically in `TestMain`.
- Spawn the plugin subprocess and verify stdio connectivity and correct JSON/text output formats for SEO tools (`seo_analyze_niche`, etc.).

---

### CLI Subcommands Integration Suite

#### [NEW] [subcommand_integration_test.go](file://../../internal/loop/subcommand_integration_test.go)

- Create a test file utilizing the `//go:build integration` tag to verify the orchestrating subcommands end-to-end.
- Compile both `powerword` and `pw-mcp-critic` binaries to a temporary test directory.
- Test `powerword review`:
  - Initialize a temporary Git repository.
  - Write a mock plan markdown file that has a conformance error, then run the compiled CLI: `./powerword review --local` and assert failure.
  - Run with the `--fix` flag and assert success and auto-fixing of plan paths.
  - Launch the compiled `pw-mcp-critic` server as an MCP plugin via the CLI configuration, and execute `./powerword review --local` to verify the critic verdict accepts/rejects.
- Test `powerword link-issue`:
  - Build a small, static Go helper binary that acts as a stub `gh` command (which prints a preset JSON issue/PR output when called with specific arguments like `gh issue view`).
  - Add the directory containing the stub `gh` command to the test runner's environment `PATH`.
  - Run the compiled `./powerword link-issue` on a dirty workspace branch and verify it processes and updates the plans and remote issue references without error.

---

## Verification Plan

### Automated Tests

- Run command: `make test-integration`
- Verify that all existing and new subprocess/CLI tests pass successfully.
