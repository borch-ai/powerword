# plan: Task 4.17: Client-Side Offline Telemetry Cache (`pkg/telemetry`)

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

## Goal Description

Extend the shareable `pkg/telemetry` package to support local offline queuing of telemetry events. When the Powerword CLI is unable to reach the Lighthouse server (e.g. offline, DNS resolution failure, network timeout), the events are cached locally in a spool file. On subsequent successful runs, queued events are automatically synchronized to Lighthouse as a batch.

## User Review Required

> [!IMPORTANT]
> **No-blocking Flush on Exit**:
> Synchronization of spooled offline events must happen asynchronously (or during CLI initialization) so it doesn't add latency to the command execution exit loop.
>
> **Queue Format**:
> We propose using a lightweight JSON Lines (`telemetry_spool.jsonl`) file in `~/.local/share/powerword/` to record offline events, preventing CGO or heavy database dependency issues.

---

## Proposed Changes

### Telemetry Subpackage (`pkg/telemetry`)

#### [MODIFY] [lighthouse.go](file://../../pkg/telemetry/lighthouse.go)
* Modify `LighthouseAdapter.Submit` to detect connection/DNS failures.
* Implement `spoolOfflineEvent(e TelemetryEvent)` which appends the event to `~/.local/share/powerword/telemetry_spool.jsonl`.
* Implement `SyncSpooledEvents(ctx context.Context)`:
  * Reads and parses all events in the spool file.
  * Submits them via a batch telemetry endpoint (e.g. `/api/telemetry/batch`).
  * Clears or rotates the spool file upon successful submission.

#### [MODIFY] [telemetry.go](file://../../pkg/telemetry/telemetry.go)
* Wire `SyncSpooledEvents` to run asynchronously on CLI initialization (e.g. when `SubmitToLighthouse` is first called or during `RunLoop` initialization).

---

## Verification Plan

### Automated Tests
* Create unit tests in `pkg/telemetry/lighthouse_test.go`:
  * Verify that a network failure to the Lighthouse adapter triggers writing to the local spool file.
  * Verify that `SyncSpooledEvents` reads the spool, posts a batch request, and truncates the file.

### Manual Verification
1. Run Powerword while pointing `LIGHTHOUSE_URL` to an invalid port (e.g. `http://localhost:9999`) and verify that execution metrics are written to the spool file.
2. Start a mock server, update `LIGHTHOUSE_URL` to the mock server, and verify that the spooled events are successfully flushed to the server on the next CLI execution.
