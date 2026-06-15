# plan: Task 6.15: End-to-End Pipeline & MCP Integration Testing Suite

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.0%

Implement a dedicated suite of integration tests (utilizing the `//go:build integration` tag) to verify the compiled binary (`powerword`) execution flows and real Model Context Protocol (MCP) transport connectivity.

## User Review Required

> [!NOTE]
> **Build Tag Separation**:
> These integration tests will be excluded from the default `go test ./...` command to maintain the speed of local unit test cycles and prevent test coverage verification issues under `make check-coverage`. They will be executed via a dedicated target (e.g. `make test-integration` or `go test -tags=integration ./...`).

## Proposed Changes

### LLM Client Component

#### [MODIFY] [openai.go](file://../../pkg/llm/openai.go)
- Update `NewOpenAIClient(apiKey string, modelName string)` to check if the `OPENAI_BASE_URL` environment variable is defined and use it as a custom base URL.

---

### Integration Testing Component

#### [NEW] [cli_integration_test.go](file://../../internal/loop/cli_integration_test.go)
- Create a test file utilizing the `//go:build integration` tag to verify the compiled `powerword` binary.
- Build the binary dynamically to a temporary path during `TestMain`.
- Verify the following CLI flows:
  * Running in headless JSON output mode (`--json`) with mocked LLM environment variables.
  * Loading and unmarshaling a custom `powerword.toml` configuration containing token pricing.
  * Correct CLI exit codes for success, configuration parsing error, and connection timeouts.
  * Resuming past session state files from a temporary directory using `--session`.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-fs/main_integration_test.go)
- Create a test file utilizing the `//go:build integration` tag to test native Go FS MCP plugin execution.
- Compile the native plugin dynamically.
- Spawn the plugin subprocess and establish a real `stdio` MCP transport connection.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-epub/main_integration_test.go)
- Create a test file utilizing the `//go:build integration` tag to test native Go EPUB MCP plugin execution.
- Compile the native plugin dynamically.
- Spawn the plugin subprocess and establish a real `stdio` MCP transport connection.

---

### Makefile

#### [MODIFY] [Makefile](file://../../Makefile)
- Add a new `test-integration` target:
  ```makefile
  test-integration:
  	go test -v -tags=integration ./...
  ```

---

## Verification Plan

### Automated Tests
- Run command: `make test-integration`
- Verify that both CLI pipeline tests and real stdio MCP plugin tests pass.
