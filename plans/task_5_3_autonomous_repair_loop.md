# Task 5.3: Autonomous Review-Repair Orchestrator

Design a unified ReAct execution loop that pushes code, waits asynchronously for webhook alerts, parses incoming review comments, and resolves them automatically through local test and lint checks.

## User Review Required

> [!CAUTION]
> **Loop Iteration Boundaries**:
> To prevent infinite loops and runaway API costs, we will enforce a strict execution limit of **5 iterations** per repair task before stopping and prompting for human intervention.

> [!IMPORTANT]
> **Human Interception Hatch**:
> The user can press `Ctrl+C` at any time to abort the autonomous loop. We will gracefully restore the workspace state using git stash/reset on abort.

## Proposed Changes

### Orchestration Loop

#### [NEW] [repair.go](file:///Users/human/code/powerword/internal/review/repair.go)
- Implement state manager to execute local test runners (`make test`), check code linting (`make lint`), and apply LLM patches.
- Connect with MCP webhook event broker to pause execution state and block on remote events.

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Evolve the reasoning loop to coordinate Git status, webhook callbacks, and local validation outputs.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/loop/...`
- Unit tests verifying:
  - Loop boundary limits (hard stop after 5 iterations).
  - Git staging and commit runner mock checks.
  - Verification that local test failures trigger repair turns.

### Manual Verification
- Cause a deliberate test failure on a branch.
- Run `powerword run --autonomous` and verify that the agent identifies the failure, edits the files, checks that tests pass, and commits/pushes the fix.
