# plan: Task 6.1: Git-Backed Workspace Rollbacks

**Status:** Open (Issue #TBD)

This task implements automated workspace rollback mechanisms using Git before and after an autonomous agent execution loop runs. If the agent fails testing or compilation gates, its changes are rolled back to keep the local working copy clean.

## User Review Required

> [!WARNING]
> This requires the CLI to run Git operations on the host workspace. Users should configure whether Powerword has permission to perform destructive git checkout/reset operations.

## Proposed Changes

### Core Execution Loop

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- [ ] Take a snapshot (Git stash or temporary branch/commit) before initiating the agent loop.
- [ ] If the agent loop exits with a failure status, automatically run git commands to rollback changes.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/loop/...` to verify rollback triggers under success/failure conditions.

### Manual Verification
- [ ] Start an autonomous repair session on a project with a failing test.
- [ ] Verify that files are restored back to their original state on completion or loop timeout.
