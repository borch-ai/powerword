# Task 3.1: Standard Native Plugins (FS, Git, Shell)

**Status:** Completed (Issue #51)


Author a suite of native Go MCP servers to expose filesystem manipulation, Git introspection, and secure shell execution capabilities to the Powerword core process.

## User Review Required

> [!WARNING]
> Shell Execution Security: Running raw shell commands poses extreme risks. The Shell MCP server must restrict execution paths, prevent command injections, and require explicit terminal authorization by default.

> [!IMPORTANT]
> Filesystem Sandbox: The FS MCP server must restrict reads/writes to the configured workspace root to prevent unintended access to sensitive files (e.g. `~/.ssh/`, `/etc/passwd`).

## Resolved Open Questions

> [!TIP]
> **Native Go Prioritization:**
> 1. **Shell Deny-List**: Hardcoded `rm, mkfs, dd, sudo, su, shutdown, reboot, poweroff, halt, format, chmod, chown` in a deny list for safety.
> 2. **Git Implementation**: Used `github.com/go-git/go-git/v5` for repository interaction, supplemented by `github.com/sergi/go-diff` to implement `git_diff` string comparison manually since `go-git` does not output textual diffs out-of-the-box.
> 3. **Search Grep**: Used Go standard library `regexp` with `filepath.WalkDir` for safe sandbox-aware regex matching.

## Proposed Changes

### Native Go MCP Servers

#### [NEW] [pw-mcp-fs](../cmd/pw-mcp-fs/main.go)
- Separate executable (or internal command mode).
- Implements MCP schema for:
  - `read_file(path)`
  - `write_file(path, content)`
  - `list_directory(path)`
  - `search_grep(pattern, path)`
- Sandbox check: prevents reading/writing outside of configured workspace root.
- Implementation: Uses `github.com/modelcontextprotocol/go-sdk/mcp`. `search_grep` uses `filepath.WalkDir` and `regexp`.

#### [NEW] [pw-mcp-git](../cmd/pw-mcp-git/main.go)
- Implements MCP schema for git repository operations:
  - `git_status()`
  - `git_diff()`
  - `git_log(limit)`
  - `git_commit(message)`
- Interacts with git repositories using the pure Go library `github.com/go-git/go-git/v5`.

#### [NEW] [pw-mcp-shell](../cmd/pw-mcp-shell/main.go)
- Implements safe shell commands running:
  - `run_command(command, args[])`
- Checks commands against a hardcoded deny-list (`rm, mkfs, dd, sudo, su, shutdown, reboot, poweroff, halt, format, chmod, chown`) and blocks them. Validates workspace directory matching.

---

## Verification Plan

### Automated Tests
- Test FS path resolution logic to verify sandbox escaping attempts (e.g. `../../etc/passwd`) are blocked and return standard errors.
- Test shell command parser with injection sequences to ensure execution safety checks trigger correctly.
- Add thorough unit tests for the MCP handler logic using `mcp.NewInMemoryTransports()`.
- Ensure tests maintain the required 91% code coverage threshold for `internal/...` core packages as enforced by CI.

### Manual Verification
- Build the plugin binaries using `go build ./cmd/pw-mcp-fs`, `go build ./cmd/pw-mcp-git`, and `go build ./cmd/pw-mcp-shell`.
- Launch the built binaries independently via terminal stdio or using an external testing tool like the `mcp-inspector`.
- Run Powerword end-to-end using the updated `powerword.toml` (which configures the servers via `go run ./cmd/...`) and prompt the LLM to read a file, check the git status, and run a safe shell command to verify the full toolchain integration.
