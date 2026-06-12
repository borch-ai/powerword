# plan: Task 6.2: Token & Cost Budgeting Guardrails

**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.0% (statements)

This task adds user-defined token and cost budgeting guardrails to prevent infinite loops and runaway API expenditures during long-running agent execution runs.

## User Review Required

> [!IMPORTANT]
> The budgeting guardrails will strictly abort the execution loop once limits are reached. Users must be able to configure default safety limits globally in `powerword.toml` and override them via CLI parameters.

## Proposed Changes

### Config and Telemetry

#### [MODIFY] [config.go](../../pkg/config/config.go)
- [x] Add config options `max_cost` and `max_tokens` (input/output/cached).

#### [MODIFY] [telemetry.go](../../pkg/telemetry/telemetry.go)
- [x] Add checks within the telemetry tracking logic to verify if the current session costs have crossed the defined budget threshold.

#### [MODIFY] [loop.go](../../internal/loop/loop.go)
- [x] Integrate budget check into the loop step validator and return a distinct budget exhaustion error.

---

## Verification Plan

### Automated Tests
- [x] Run `go test ./pkg/telemetry/...` and `go test ./internal/loop/...` testing with mock token budgets.

### Manual Verification
- [x] Run `powerword` with `--max-cost 0.01` and verify that the run is aborted early when tool executions or prompt completions trigger costs exceeding $0.01.
