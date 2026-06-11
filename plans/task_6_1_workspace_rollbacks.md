# plan: Task 6.1: Git-Backed Workspace Rollbacks

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91.0%

This task implements automated workspace rollback mechanisms using Git before and after an autonomous agent execution loop runs. If the agent fails testing or compilation gates, its changes are rolled back to keep the local working copy clean.

## User Review Required

> [!WARNING]
> This requires the CLI to run Git operations on the host workspace. Users should configure whether Powerword has permission to perform destructive git checkout/reset operations.

## Proposed Changes

### Core Execution Loop & Rollback Engine

#### [MODIFY] [config.go](file:///Users/human/code/powerword/pkg/config/config.go)
- [x] Add `GitRollback` field (`git_rollback`) to Viper settings.

#### [MODIFY] [root.go](file:///Users/human/code/powerword/pkg/config/root.go)
- [x] Add `--git-rollback` flag to the root CLI command runner.

#### [NEW] [rollback.go](file:///Users/human/code/powerword/internal/loop/rollback.go)
- [x] Implement `WorkspaceSnapshot` capturing current HEAD commit, unstaged changes, and untracked files.
- [x] Implement `Restore` restoring HEAD, clean untracked files, and popping user stashes.
- [x] Implement `CleanUp` dropping snapshot stashes if successful.

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- [x] Take a snapshot (Git stash or temporary branch/commit) before initiating the agent loop.
- [x] If the agent loop exits with a failure status, automatically run git commands to rollback changes.

#### [MODIFY] [repair.go](file:///Users/human/code/powerword/internal/review/repair.go)
- [x] Integrate snapshot capture and restore behaviors into the autonomous repair loop.

#### [NEW] [rollback_test.go](file:///Users/human/code/powerword/internal/loop/rollback_test.go)
- [x] Add comprehensive unit tests covering clean repo, dirty repo, stashes, and Git mock errors.

#### [MODIFY] [repair_test.go](file:///Users/human/code/powerword/internal/review/repair_test.go)
- [x] Add unit tests for autonomous loop rollback integration.

---

## Verification Plan

## Verification Plan

### Automated Tests
- [x] Run `go test ./internal/loop/...` to verify rollback triggers under success/failure conditions.
- [x] Run `make check-coverage` to assert coverage exceeds the 91.0% threshold.

### Manual Verification
- [x] Start an autonomous repair session on a project with a failing test.
- [x] Verify that files are restored back to their original state on completion or loop timeout.
