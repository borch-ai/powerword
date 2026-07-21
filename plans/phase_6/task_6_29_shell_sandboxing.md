# plan: Task 6.29: Host-Level Containerized & Profile-Based Sandboxing (`pw-mcp-shell`)

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

## Goal Description

Introduce containerized and profile-based host sandboxing to the native Go shell MCP server (`pw-mcp-shell`). Developers will be able to restrict execution context limits of shell tools, specifying if the tool can modify files outside the workspace, communicate with external network domains, or execute process-spawning operations.

## User Review Required

> [!IMPORTANT]
> **OS Portability**:
> Sandboxing mechanisms differ significantly between macOS (`sandbox-exec` profiles) and Linux (user namespaces, `seccomp`, or containers). We propose defining an abstract `SandboxProvider` interface with OS-specific implementations to ensure compatibility.
>
> **Container Fallback**:
> We propose using local Docker or Podman containers as a default fallback option when local OS sandboxing is unavailable.

---

## Proposed Changes

### Plugins Component (`pw-mcp-shell`)

#### [NEW] [sandbox.go](file://../../cmd/pw-mcp-shell/sandbox.go)

* Define `SandboxProfile` holding restrictions:
  * `AllowNetwork bool`
  * `AllowedPaths []string`
  * `WritablePaths []string`
* Define `SandboxProvider` interface:
  * `WrapCommand(ctx context.Context, cmd *exec.Cmd, profile SandboxProfile) (*exec.Cmd, error)`
* Implement `macOSProvider` using `/usr/bin/sandbox-exec` and dynamically generated `.sb` scheme profiles.
* Implement `linuxProvider` utilizing standard namespaces (`unshare`) or `seccomp` system call filters.
* Implement `containerProvider` wrapping execution inside a transient `docker run` command mount.

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-shell/main.go)

* Modify the shell command execution tool to parse sandbox parameters and run the command via the active `SandboxProvider`.

---

## Verification Plan

### Automated Tests

* Create unit tests in `cmd/pw-mcp-shell/sandbox_test.go`:
  * Verify that a command executing network calls (e.g. `curl`) is blocked when `AllowNetwork = false`.
  * Verify file writes outside allowed paths are rejected.

### Manual Verification

1. Run `pw-mcp-shell` under a strict read-only profile.
2. Execute a tool call to write to `/tmp/unsafe` and verify that the operation is blocked by the sandbox.
