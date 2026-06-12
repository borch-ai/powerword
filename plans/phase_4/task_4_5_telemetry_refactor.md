# plan: Task 4.5: Shareable Telemetry Subpackage Refactor

**Status:** Completed (Implemented under Go 1.26.4)
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%

Refactor the telemetry and cost accounting structures from internal packages into a public, dependency-free subpackage inside `powerword`. Additionally, update `powerword`'s module name to `github.com/borch-ai/powerword` so it is importable by Pithos.

## User Review Required

> [!WARNING]
> **Module Path Renaming**:
> Changing the module path from `powerword` to `github.com/borch-ai/powerword` is a major refactoring that requires updating all internal package imports across the entire `powerword` repository.

---

## Proposed Changes

### Module Setup

#### [MODIFY] [go.mod](file:///Users/human/code/powerword/go.mod)
- Update the module declaration from `module powerword` to `module github.com/borch-ai/powerword`. (Already complete in base codebase)

#### [MODIFY] [All Go files in powerword]
- Update all internal package import statements from `"powerword/internal/..."` to `"github.com/borch-ai/powerword/internal/..."`. (Already complete in base codebase)

### Telemetry Component

#### [NEW] [telemetry.go](file:///Users/human/code/powerword/pkg/telemetry/telemetry.go)
- Create a new, dependency-free subpackage `pkg/telemetry` containing:
  * `ModelUsage` struct (token counts).
  * `UsageTracker` struct.
  * `ModelPricing` configuration mapping (relocated from config files).
  * Prefix matching and cost calculation logic (transferred from `pkg/llm/telemetry.go`).

#### [DELETE] [telemetry.go](file:///Users/human/code/powerword/pkg/llm/telemetry.go)
- Remove the old telemetry implementation to avoid duplication.

#### [NEW] [telemetry_test.go](file:///Users/human/code/powerword/pkg/telemetry/telemetry_test.go)
- Relocate and adapt the unit tests from `pkg/llm/telemetry_test.go` to test the new package under the `telemetry` namespace.

#### [DELETE] [telemetry_test.go](file:///Users/human/code/powerword/pkg/llm/telemetry_test.go)
- Remove old test file.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./pkg/telemetry/... ./internal/...`
- Ensure all tests pass and that there are no package cycle import loops.
