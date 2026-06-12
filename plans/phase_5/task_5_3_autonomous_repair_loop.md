# plan: Task 5.3: Autonomous Review-Repair & Issue Comment Orchestration

**Status:** Complete (Issue #57)

**Final Configuration:** Added `Autonomous` and `Issue` settings to the main `config.toml` structure.
**Go Version Used:** Go 1.26.4
**Coverage Validation:** Unit tests added in `internal/review/repair_test.go` holding the codebase at 91.2% total test coverage.

Evolve the ReAct execution loop to read tasks directly from GitHub issues, run local checks, push changes, and publish progress updates and state reports back as issue comments.

## User Review Required

> [!CAUTION]
> **Loop Iteration Boundaries**:
> To prevent infinite loops and runaway API costs, we will enforce a strict execution limit of **5 iterations** per repair task before stopping and prompting for human intervention.

> [!IMPORTANT]
> **Human Intervention Hatch**:
> The user can press `Ctrl+C` at any time to abort the autonomous loop. We will gracefully restore the workspace state using git stash/reset on abort.

## Proposed Changes

### Orchestration Loop

#### [NEW] [repair.go](file:///Users/human/code/powerword/internal/review/repair.go)
- Implement state manager to execute local test runners (`make test`), check code linting (`make lint`), and apply LLM patches.
- Add GitHub comment posting logic using `gh issue comment <id> --body "..."` or direct API client.
- Automatically update issue progress checklists and change state status to "closed" on task completion.

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Evolve the reasoning loop to coordinate Git status, webhook callbacks, local validation outputs, and issue comment updates.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/loop/...`
- Unit tests verifying:
  - Loop boundary limits (hard stop after 5 iterations).
  - Issue comment payload generation (properly formatting task checklists).
  - Verification that local test failures trigger repair turns.

### Manual Verification
- Cause a deliberate test failure on a branch.
- Run `powerword run --autonomous` and verify that the agent identifies the failure, edits the files, checks that tests pass, logs a comment stating progress, and commits/pushes the fix.

