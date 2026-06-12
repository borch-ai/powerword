# plan: Task 2.2: Stdio Transport Layer & Server Lifecycle

**Status:** Completed (Issue #45)
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%

Implement the process control logic to launch, supervise, communicate with, and terminate external MCP servers using standard input/output (stdio) streams as the primary message transport mechanism.

## User Review Required

> [!WARNING]
> Process Leakage: If Powerword crashes, we must ensure child processes (the external MCP servers) are cleanly terminated. We must register signal handlers and implement robust cleanup routines.

## Proposed Changes

### Stdio Transport & Process Management

#### [NEW] [process.go](file:///Users/human/code/powerword/internal/mcp/process.go)
- Defines a process supervisor `ServerProcess` that wraps an `os/exec.Cmd`.
- Sets up pipes for `stdin`, `stdout`, and `stderr`.
- Connects the pipes to the `go-sdk`'s stdio transport adapter.
- Configures background workers to log server `stderr` outputs for diagnostic purposes.

#### [NEW] [lifecycle.go](file:///Users/human/code/powerword/internal/mcp/lifecycle.go)
- Implements process registry cleanup methods.
- Hooks into application exit signals (e.g. `SIGINT`, `SIGTERM`, `SIGHUP`) to execute graceful shutdown commands (MCP exit notifications) and kill unresponsive subprocesses.

---

## Verification Plan

### Automated Tests
- Spawn an echo subprocess script and test sending/receiving JSON-RPC messages via standard input/output streams.
- Test signal interception logic using controlled test sub-processes.

### Manual Verification
1. Create a `powerword.toml` configuration file in the project root with the following content to register a sample MCP server (e.g., `server-everything`):
   ```toml
   [servers.everything]
   command = "npx"
   args = ["-y", "@modelcontextprotocol/server-everything"]
   ```
2. Run the powerword CLI with verbose logging so we can observe the background workers and stderr outputs:
   ```bash
   POWERWORD_VERBOSE=true go run ./cmd/powerword "Hello, world!"
   ```
3. Verify that the CLI output logs indicate the `everything` server has started and the background `stderr` monitoring goroutine is running (you should see MCP server initialization logs if any).
4. Terminate Powerword via Ctrl+C while it is running, and verify the console outputs that the graceful shutdown sequence was triggered and the child process was closed correctly without leaving zombie processes.

---

## Final Implementation Details
- **Go Version:** Go 1.26.4
- **Configuration:** Added `ServerConfig` (Command, Args, Env) and a `Servers` map to `config.Config` inside `internal/config/config.go` for dynamic MCP server definitions.
- **Process Supervision:** `internal/mcp/process.go` implements `ServerProcess` which captures stdin/stdout/stderr pipes, wrapping the streams in `mcpsdk.IOTransport`. A background goroutine reads `stderr` to `log.Printf` for diagnostics.
- **Graceful Shutdown:** `internal/mcp/lifecycle.go` exports `ProcessManager` which listens for `os.Interrupt` and `syscall.SIGTERM`. On receipt, it issues `client.Close()`, sends `os.Interrupt` to the child process, and waits. If the child process exceeds a 5-second timeout, `SIGKILL` is issued.
- **Testing:** Implemented comprehensive unit tests for config map updates and process lifecycle/signal hooking. Maintained 91% global test coverage threshold (`make check-coverage` verified).
