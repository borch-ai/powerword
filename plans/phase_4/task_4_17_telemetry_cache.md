# plan: Task 4.17: Client-Side Offline Telemetry Cache (`pkg/telemetry`)

**Status:** Completed
**Go Version:** 1.26.5
**Date Completed:** 2026-07-15
**Unit Test Coverage:** 91.0%

This task extends the shareable `pkg/telemetry` package to support local offline queuing of telemetry events. When the Powerword CLI (or any importing client) is unable to reach the Lighthouse server, the events are spooled locally. On subsequent successful runs, queued events are synchronized to Lighthouse in a batch.

## User Review Required

> [!IMPORTANT]
> **Race-Free Concurrency via Atomic Rename & File Lock Coordination**:
> Multiple tool commands or concurrent sibling runs (e.g. Kiln, Pithos) can execute simultaneously. To prevent race conditions where one process is writing a new event to the spool while another process is reading/flushing and truncating it, we use an **atomic rename strategy** coupled with **lock file coordination**:
> 1. Writes to the spool file, renames, and rollbacks are protected by a cross-process lock file (`telemetry_spool.jsonl.lock`) using `os.O_EXCL` file creation.
> 2. When syncing, we acquire the lock, rename `telemetry_spool.jsonl` to `telemetry_spool_sync_<timestamp>.jsonl` (which is atomic), and release the lock.
> 3. The syncing process reads this renamed file, posts its contents via a batch API, and deletes it on success.
> 4. If posting fails, we append the events back to `telemetry_spool.jsonl` under the lock.
> 5. Any concurrent processes writing new telemetry during the sync will be blocked until the rename completes. Once the lock is released, they will simply create a new `telemetry_spool.jsonl` and append to it without interference, ensuring zero event loss.
> 6. We enforce a maximum size limit of **5 MB** on the spool file to bound disk usage and subsequent sync latency.
> 7. The retryability checks are based on structured HTTP error responses (`HTTPError`), context cancellation, and `net.Error` transport timeouts, with a fallback string parser for backwards compatibility.

---

## Proposed Changes

### Telemetry Package (`pkg/telemetry`)

#### [MODIFY] [lighthouse.go](file://../../pkg/telemetry/lighthouse.go)
- Add `HTTPError` struct and return it from `Submit` and `SubmitBatch` on non-2xx status codes.
- Implement `withFileLock(spoolPath string, staleThreshold time.Duration, action func() error) error` helper implementing lock file coordination via an `O_EXCL` lock file. Enforces configurable staleness checks and dynamic retry sleep durations based on `lockTimeout` to eliminate test flakiness.
- Implement `generateEventID` to assign unique event IDs using `crypto/rand`, falling back to a thread-safe deterministic schema on entropy error. Assign event ID once at the start of `SubmitToLighthouse` to ensure consistent ID spooling.
- Add `SubmitBatch(ctx context.Context, events []TelemetryEvent) error` to `LighthouseAdapter` to submit an array of events to `/api/telemetry/batch`.
- Implement `spoolOfflineEvent(e TelemetryEvent)`:
  - Resolves standard directory `~/.local/share/powerword/` using `os.UserHomeDir()` with secure permissions (`0700`).
  - Appends the event as a single line JSON-marshaled payload into `telemetry_spool.jsonl` with a trailing newline (`\n`).
  - Limits spool file size to 5 MB, accounting for the appended event size even when the file does not exist yet.
- Implement `SyncSpooledEvents(ctx context.Context)`:
  - Runs once per process under `sync.Once`.
  - Uses `withFileLock` to safely rename `telemetry_spool.jsonl` to `telemetry_spool_sync_<timestamp>.jsonl`.
  - Configures `bufio.Scanner` with a 5 MB buffer limit to handle large lines.
  - Sends events in batches of 100 via `SubmitBatch`.
  - Deletes the temporary sync file on success (or renames corrupt lines to `.corrupt`).
  - Appends events back to `telemetry_spool.jsonl` on failure under lock.
- Refactor `isRetryableError(err error) bool` to perform structured checks for context cancellation, `net.Error`, and `HTTPError`, with a fallback string parser for raw errors.

#### [MODIFY] [telemetry.go](file://../../pkg/telemetry/telemetry.go)
- No changes required.

#### [MODIFY] [lighthouse_test.go](file://../../pkg/telemetry/lighthouse_test.go)
- Add `TestMain` override to set `lockTimeout = 5 * time.Millisecond` to keep unit test suites running in milliseconds.
- Add `TestLighthouseAdapter_SubmitBatch_Success` and `TestLighthouseAdapter_SubmitBatch_Error`.
- Add unit tests verifying spooling, atomic rename/locking, batching, error rollback, file size capping, stale lock recovery, and structured error retry categorizations.

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
