# plan: Task 6.31: Dynamic Plugin Auto-Discovery & Hot Reloading

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

## Goal Description

Introduce dynamic plugin auto-discovery and config hot-reloading. Powerword will automatically discover local MCP servers (binaries or manifest files) placed in a designated user directory (e.g. `~/.config/powerword/plugins.d/`) on startup. It will also watch the configuration file for changes, automatically mounting or dismantling MCP processes without disrupting active prompt loops or losing state.

## User Review Required

> [!IMPORTANT]
> **FSNotify Portability**:
> Config watching relies on `fsnotify`. We must ensure file event notifications are portable and do not cause resource leaks or system crashes on specific OS variants.
>
> **Active Process Cleanup**:
> When a plugin is hot-reloaded or removed, the active child process must be gracefully terminated (via SIGTERM and fallback SIGKILL) before mounting any new instance, preventing orphaned processes.

---

## Proposed Changes

### Configuration Component (`pkg/config`)

#### [MODIFY] [config.go](file://../../pkg/config/config.go)

* Implement `WatchConfig(ctx context.Context, onChange func(*Config))` using Viper's config watcher, executing the callback in a safe goroutine.
* Add auto-discovery scan helper:
  * Scans directory `~/.config/powerword/plugins.d/` for `.json` / `.toml` plugin definitions.
  * Dynamically populates `Config.Servers`.

### Process Lifecycle Component (`internal/mcp`)

#### [MODIFY] [process.go](file://../../internal/mcp/process.go)

* Implement `ProcessManager.Reload(name string, srvCfg ServerConfig)`:
  * Gracefully shuts down the existing server connection.
  * Spawns the new process configuration.
  * Re-mounts the client in the execution registry.

---

## Verification Plan

### Automated Tests

* Create unit tests in `pkg/config/config_test.go` and `internal/mcp/process_test.go`:
  * Mock file modifications and verify that configuration reload callbacks are executed.
  * Verify `Reload` terminates existing subprocesses and successfully restarts target processes with new configurations.

### Manual Verification

1. Create a dummy plugin config file in `~/.config/powerword/plugins.d/`.
2. Start Powerword, update `powerword.toml` in another terminal, and observe hot-reload log messages confirming process re-creation.
