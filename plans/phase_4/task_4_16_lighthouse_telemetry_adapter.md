# plan: Task 4.16: Lighthouse Telemetry HTTP Adapter (`pkg/telemetry`)

**Status:** Completed
**Date Completed:** 2026-07-07
**Go Version:** 1.26.5
**Unit Test Coverage:** 91.3%

Extend `pkg/telemetry` with an optional HTTP adapter that submits session execution metrics to a Lighthouse `POST /api/telemetry` endpoint on command exit. All sibling tools (Pithos, Aeolian, Kiln) that import `pkg/telemetry` get this capability for free once merged.

---

## Motivation

Currently, token counts and cost data are written to `manifest.json` (Pithos) and `audit.jsonl` (Kiln) and computed locally. There is no unified, cross-project view of where money is being spent. Lighthouse provides this unified view, but only if the tools push their metrics to it.

This task adds the push mechanism as a background, non-blocking goroutine — zero impact on CLI exit latency, graceful no-op if `LIGHTHOUSE_URL` is unset.

---

## User Review Required

This plan introduces a new `LighthouseAdapter` in `pkg/telemetry` and wires it into the existing `Flush()` call. Key design decisions for review:

- **Non-blocking fire-and-forget** — the HTTP call runs in a detached goroutine with a 3-second deadline. Failure is logged as a warning, never propagated to the caller.
- **Zero-config no-op** — if `LIGHTHOUSE_URL` is unset the adapter does nothing; no changes are required in Pithos, Aeolian, or Kiln.
- **Auth optional** — `LIGHTHOUSE_API_KEY` is read if present; local dev instances work without it.

---

## Proposed Changes

### `pkg/telemetry/lighthouse.go` [NEW]
```go
// LighthouseAdapter submits a TelemetryEvent to a Lighthouse collector.
// It runs in a detached goroutine and must not block the caller.
type LighthouseAdapter struct {
    URL    string
    APIKey string
    Client *http.Client
}

func (a *LighthouseAdapter) Submit(ctx context.Context, e TelemetryEvent)
```

`TelemetryEvent` struct:
```go
type TelemetryEvent struct {
    Project    string            `json:"project"`
    Command    string            `json:"command"`
    Stage      string            `json:"stage"`
    DurationMs int64             `json:"duration_ms"`
    CostUSD    float64           `json:"cost_usd"`
    TokensIn   int64             `json:"tokens_in"`
    TokensOut  int64             `json:"tokens_out"`
    Meta       map[string]string `json:"meta_json,omitempty"`
}
```

### `pkg/telemetry/telemetry.go` [MODIFY]
Add `SubmitToLighthouse(event TelemetryEvent)` convenience function that:
1. Reads `LIGHTHOUSE_URL` env var. If unset, returns immediately (no-op).
2. Reads `LIGHTHOUSE_API_KEY` env var for the Bearer token.
3. Spawns `LighthouseAdapter.Submit()` in a goroutine with a 3-second context deadline.
4. Logs a warning (not an error) if the HTTP request fails — never propagates errors to caller.

---

## Usage by Callers

No changes required in Pithos, Aeolian, or Kiln code. Each tool's existing telemetry flush call becomes:

```go
// Existing call (unchanged)
telemetry.Flush(manifest)

// After this task: Flush() internally calls SubmitToLighthouse() if LIGHTHOUSE_URL is set
```

---

## Environment Variables

| Variable | Description |
|---|---|
| `LIGHTHOUSE_URL` | Base URL of the Lighthouse instance (e.g. `http://localhost:8080`). If unset, the adapter is a no-op. |
| `LIGHTHOUSE_API_KEY` | Bearer token for the Lighthouse collector. Optional — if unset, the request is sent without auth (suitable for local instances). |

---

## Verification Plan

### Automated Tests
- `TestLighthouseAdapter_Submit_Success`: Start a mock HTTP server, call `Submit()`, verify the correct JSON body is posted to `/api/telemetry`.
- `TestLighthouseAdapter_Submit_NoURL`: Verify that `SubmitToLighthouse` returns immediately without spawning a goroutine when `LIGHTHOUSE_URL` is unset.
- `TestLighthouseAdapter_Submit_Timeout`: Verify that the adapter times out after 3 seconds and does not block.

### Manual Verification
- Set `LIGHTHOUSE_URL=http://localhost:8080` in the test env.
- Run `pithos brew --dry-run` (Task 5.37, after it ships).
- Verify a row appears in the Lighthouse `system_telemetry` table with `project="pithos"`.
