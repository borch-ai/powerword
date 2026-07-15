# plan: Task 4.17: Client-Side Offline Telemetry Cache (`pkg/telemetry`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-07-15
**Unit Test Coverage:** 91.0%

This task extends the shareable `pkg/telemetry` package to support local offline queuing of telemetry events. When the Powerword CLI (or any importing client) is unable to reach the Lighthouse server, the events are spooled locally. On subsequent successful runs, queued events are synchronized to Lighthouse in a batch.

## User Review Required

> [!IMPORTANT]
> **Race-Free Concurrency via Atomic Rename**:
> Multiple tool commands or concurrent sibling runs (e.g. Kiln, Pithos) can execute simultaneously. To prevent race conditions where one process is writing a new event to the spool while another process is reading/flushing and truncating it, we will use an **atomic rename strategy**:
> 1. When syncing, we rename `telemetry_spool.jsonl` to `telemetry_spool_sync_<timestamp>.jsonl` (which is atomic on POSIX).
> 2. The syncing process reads this renamed file, posts its contents via a batch API, and deletes it on success.
> 3. If posting fails, we append the events back to `telemetry_spool.jsonl`.
> 4. Any concurrent processes writing new telemetry during the sync will simply create a new `telemetry_spool.jsonl` and append to it without interference.

---

## Proposed Changes

### Telemetry Package (`pkg/telemetry`)

#### [MODIFY] [lighthouse.go](file://../../pkg/telemetry/lighthouse.go)
- Add `SubmitBatch(ctx context.Context, events []TelemetryEvent) error` to `LighthouseAdapter` to submit an array of events to `/api/telemetry/batch`.
- Implement `spoolOfflineEvent(e TelemetryEvent)`:
  - Resolves standard directory `~/.local/share/powerword/` using `os.UserHomeDir()`.
  - Appends the event as a single line JSON-marshaled payload into `telemetry_spool.jsonl` with a trailing newline (`\n`).
  - Ensures parent directories are created with `0750` permissions and the file is opened/created with `0600` permissions.
- Implement `SyncSpooledEvents(ctx context.Context)`:
  - Runs in a background goroutine during telemetry initialization (once per CLI invocation).
  - Uses `os.Rename` to atomically move `telemetry_spool.jsonl` to a temporary sync file `telemetry_spool_sync_<timestamp>.jsonl`.
  - Reads and parses all events from the temporary sync file.
  - Sends the events to Lighthouse via `SubmitBatch` (e.g. in batches of 100).
  - Deletes the temporary sync file on success.
  - Appends the events back to `telemetry_spool.jsonl` on failure.
- Modify `SubmitToLighthouse(e TelemetryEvent)` to spool the event using `spoolOfflineEvent` if `Submit` fails.
- Wire up a `sync.Once` to ensure `SyncSpooledEvents` is triggered exactly once on the first call to `SubmitToLighthouse`.

#### [MODIFY] [telemetry.go](file://../../pkg/telemetry/telemetry.go)
- No changes required if all initialization and caching logic is encapsulated inside `lighthouse.go`.

#### [MODIFY] [lighthouse_test.go](file://../../pkg/telemetry/lighthouse_test.go)
- Add `TestLighthouseAdapter_SubmitBatch_Success` and `TestLighthouseAdapter_SubmitBatch_Error`.
- Add unit tests verifying:
  - Writing events to the spool file on submission failure.
  - Atomic rename, reading, batching, and deleting on successful sync.
  - Correct rollback (re-spooling) if batch sync fails.

---

## Verification Plan

### Automated Tests
- Run tests and check coverage threshold:
  ```bash
  make test-fast
  ```
- Run specific tests under telemetry package:
  ```bash
  go test -v -race ./pkg/telemetry/...
  ```
- Verify that test coverage is maintained above 91.0%.

### Manual Verification
1. Set `LIGHTHOUSE_URL` to an invalid address (e.g., `http://localhost:9999`) and run a command. Verify that `~/.local/share/powerword/telemetry_spool.jsonl` is created and contains the telemetry event.
2. Set up a local test server mapping `/api/telemetry/batch` to accept batch events. Set `LIGHTHOUSE_URL` to the test server and run a command. Verify that the spool file is synchronized and deleted.
