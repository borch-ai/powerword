# plan: Task 6.24: Active Process Termination & Poll Cleanup on Context Cancellation

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** 
**Unit Test Coverage:** 

---

## User Review Required

> [!WARNING]
> This task ensures that context cancellations translate directly to immediate operating system process signals (e.g. `SIGKILL` or `SIGTERM`) on child processes, preventing orphan processes.

---

## Problem

When the main agent loop aborts, hits a budget limit, or is cancelled by the user, the execution context (`context.Context`) is marked as done. However, background commands spawned by Powerword plugins (such as `ffmpeg` in the `viral` plugin) or long-running status check loops (like the `Midjourney` or `Veo` status checkers) may not cleanly abort or terminate immediately. This can lead to leaked orphaned processes or redundant API consumption.

---

## Goal

Audit and enforce strict cancellation propagation:
1. Ensure all `exec.CommandContext` child processes spawned in native plugins are forcefully killed (sent appropriate termination signals) when the context is cancelled.
2. Ensure all background loops and tickers immediately stop polling remote endpoints and release their resources.

---

## Proposed Changes

### `internal/plugins/viral/`

#### [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)

- Refactor `exec.CommandContext` calls (e.g., in `runMockVideo`, `generateTTSMock`, `StitchTrailer`) to monitor the context state:
  ```go
  // Ensure that on context cancellation, we send a kill signal to the process group if it hasn't exited
  ```
- Ensure temporary files are cleaned up via `defer` in error paths.

### `internal/plugins/imagegen/`

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

- Audit `pollVeo` and Midjourney's poll loop in `GenerateImage` to verify they cancel ongoing HTTP transport requests immediately and release tickers on `ctx.Done()`.

---

## Verification Plan

### Automated Tests
- Create unit/integration tests that trigger a long command (like `sleep 10` or a slow loop) and cancel the context, asserting:
  - The command process is no longer running in the OS process list.
  - Tickers and tickers channels are fully closed and GC-able.
- Run tests:
  ```bash
  go test -v ./internal/plugins/viral/...
  go test -v ./internal/plugins/imagegen/...
  ```
- Verify code coverage:
  ```bash
  make check-coverage
  ```
