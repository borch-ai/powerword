# plan: Task 6.25: OpenTelemetry (OTel) Tracing Instrumentation

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:**
**Unit Test Coverage:**

---

## User Review Required

> [!NOTE]
> Introduces `go.opentelemetry.io/otel` dependencies to export structured tracer spans to standard monitoring collectors.

---

## Problem

In multi-turn ReAct agent reasoning loops, tracking down which subtask is bottlenecking execution, tracing tool failure paths, or diagnosing network issues is extremely difficult. Sibling systems, such as the Lamplighter dashboard, need standard structured trace data to calculate precise run statistics, latency breakdowns, and loop costs.

---

## Goal

Add OpenTelemetry (OTel) tracing instrumentation:

1. Initialize an OTel tracer provider during CLI startup, supporting an optional OTLP/HTTP or stdout collector endpoint configured via `config.toml`.
2. Instrument the core ReAct reasoning loop (`internal/loop`) with trace spans.
3. Instrument the MCP client tool execution handlers to capture input/output metadata and latency statistics.

---

## Proposed Changes

### Configuration Management

#### [MODIFY] [config.go](file://../../pkg/config/config.go)

- Add telemetry collector endpoint configuration (`telemetry_endpoint` and `telemetry_headers`).

### `pkg/telemetry/`

#### [MODIFY] [telemetry.go](file://../../pkg/telemetry/telemetry.go)

- Introduce an OTel tracer initializer helper function to setup global tracer provider.
- Setup shutdown hooks for flushing traces before CLI exit.

### Core Execution Loop

#### [MODIFY] [loop.go](file://../../internal/loop/loop.go)

- Inject `otel.Tracer` spans to trace the main agent session.
- Add child spans per ReAct iteration capturing model output metadata.

### MCP Client Integration

#### [MODIFY] [client.go](file://../../internal/mcp/client.go)

- Wrap `CallTool` calls in OTel spans recording the tool name, arguments, and return outcome.

---

## Verification Plan

### Automated Tests

- Test span generation in `loop_test.go` using an OTel in-memory span exporter, asserting:
  - Loop session span exists.
  - Iteration and tool child spans are nested correctly under the parent.
- Run tests:

  ```bash
  go test -v ./internal/loop/...
  go test -v ./pkg/telemetry/...
  ```

- Verify code coverage:

  ```bash
  make check-coverage
  ```
