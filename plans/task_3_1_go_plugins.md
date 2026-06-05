# Task 3.1: Standard Native Plugins (FS, Git, Shell)

Author a suite of native Go MCP servers to expose filesystem manipulation, Git introspection, and secure shell execution capabilities to the Powerword core process.

## User Review Required

> [!WARNING]
> Shell Execution Security: Running raw shell commands poses extreme risks. The Shell MCP server must restrict execution paths, prevent command injections, and require explicit terminal authorization by default.

## Proposed Changes

### Native Go MCP Servers

#### [NEW] [pw-mcp-fs](file:///Users/human/code/powerword/cmd/pw-mcp-fs/main.go)
- Separate executable (or internal command mode).
- Implements MCP schema for:
  - `read_file(path)`
  - `write_file(path, content)`
  - `list_directory(path)`
  - `search_grep(pattern, path)`
- Sandbox check: prevents reading/writing outside of configured workspace root.

#### [NEW] [pw-mcp-git](file:///Users/human/code/powerword/cmd/pw-mcp-git/main.go)
- Implements MCP schema for git repository operations:
  - `git_status()`
  - `git_diff()`
  - `git_log(limit)`
  - `git_commit(message)`
- Interacts with git repositories directly using Go system exec or `go-git`.

#### [NEW] [pw-mcp-shell](file:///Users/human/code/powerword/cmd/pw-mcp-shell/main.go)
- Implements safe shell commands running:
  - `run_command(command, args[])`
- Checks commands against a customizable deny-list (e.g. blocking `rm -rf /`, `mkfs`, etc.) and requires manual approval via terminal prompt before execution.

---

## Verification Plan

### Automated Tests
- Test FS path resolution logic to verify sandbox escaping attempts (e.g. `../../etc/passwd`) are blocked and return standard errors.
- Test shell command parser with injection sequences to ensure execution safety checks trigger correctly.

### Manual Verification
- Launch the `pw-mcp-fs` server independently via terminal stdio. Submit standardized JSON-RPC initialization and tool execution requests, verifying standard output response formatting.
