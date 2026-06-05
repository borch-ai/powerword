# Task 2.2: Stdio Transport Layer & Server Lifecycle

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
- Launch a Node/Python-based MCP server (like the filesystem server) via command configs.
- Terminate Powerword via Ctrl+C and verify the child process is automatically closed.
