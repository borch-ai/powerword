# plan: Task 6.6: Dry-Run Mode for Operations Plugins

**Status:** Open (Issue #TBD)

This task implements a dry-run policy for modifying commands inside database, cloud, and Kubernetes plugins.

## User Review Required

> [!CAUTION]
> Safety checking operations must prevent destructive actions (e.g., table drops or cloud resource terminations). We need to mock responses realistically so the agent can still plan actions.

## Proposed Changes

### Operations Plugins

#### [NEW] [k8s.go](file:///Users/human/code/powerword/internal/plugins/k8s/k8s.go)
- [ ] If dry-run mode is enabled, intercept cluster-modifying calls and mock the response metadata.

#### [NEW] [db.go](file:///Users/human/code/powerword/internal/plugins/db/db.go)
- [ ] Parse query syntax to block statements containing updates, deletions, or schema alterations if dry-run mode is toggled on.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/plugins/...` checks on dry-run SQL parses.

### Manual Verification
- [ ] Enable dry-run mode in database inspector config and run a query targeting modifications, verifying it gets mocked safely.
