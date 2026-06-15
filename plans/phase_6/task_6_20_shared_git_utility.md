# plan: Task 6.20: Shared Git Utility Package (pkg/gitutil)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-15
**Unit Test Coverage:** 100.0% (pkg/gitutil), 91.2% overall

Extract low-level Git commands and command execution from the internal rollback safety-net engine into a public, shared Go package (`pkg/gitutil`). This makes repository status checks, initialization, stashing, and commits accessible to sibling projects (like Pithos).

## User Review Required

> [!WARNING]
> **Host Git Requirement**:
> These utilities run Git commands directly on the host using `os/exec`. If Git is missing from the system PATH, the helpers will return clean errors that callers must handle gracefully.

---

## Proposed Changes

### Shared Git Engine

#### [NEW] [gitutil.go](file:///Users/human/code/powerword/pkg/gitutil/gitutil.go)
Create the `pkg/gitutil` package exposing clean APIs for Git operations:
* Support custom commands runner through package-level function variable `var ExecCommand = exec.CommandContext` to allow unit-test mocking.
* `func RunGitCommand(ctx context.Context, dir string, args ...string) (string, error)` (executes git commands and filters coverage warnings).
* `func IsInsideWorkTree(ctx context.Context, dir string) (bool, error)`
* `func GetHeadCommit(ctx context.Context, dir string) (string, error)`
* `func Init(ctx context.Context, dir string) error`
* `func AddAll(ctx context.Context, dir string) error`
* `func Commit(ctx context.Context, dir string, message string) error`
* `func Clean(ctx context.Context, dir string) error`
* `func ResetHard(ctx context.Context, dir string, commit string) error`
* `func StashPush(ctx context.Context, dir string, message string) error`
* `func StashApply(ctx context.Context, dir string, index int) error`
* `func StashPop(ctx context.Context, dir string, index int) error`
* `func StashDrop(ctx context.Context, dir string, index int) error`
* `func StashList(ctx context.Context, dir string) (string, error)`

#### [NEW] [gitutil_test.go](file:///Users/human/code/powerword/pkg/gitutil/gitutil_test.go)
* Add unit tests using temporary directories and mocked command executions to test success and failure paths of the newly added APIs.
* Verify coverage for `pkg/gitutil` meets the `≥91.0%` threshold.

### Rollback Refactor

#### [MODIFY] [rollback.go](file:///Users/human/code/powerword/internal/loop/rollback.go)
* Import `github.com/borch-ai/powerword/pkg/gitutil`.
* Replace internal command runners and command calls (like stashing, cleaning, and resetting) with calls to the shared `gitutil` package.
* Delete local duplicate `runGitCommand` method.

#### [MODIFY] [rollback_test.go](file:///Users/human/code/powerword/internal/loop/rollback_test.go)
* Adjust internal test mocks to hook into the new package level mocking configurations of `pkg/gitutil`.

---

## Verification Plan

### Automated Tests
* Run `go test -v ./pkg/gitutil/... ./internal/loop/...`
* Run `make check-coverage` to assert total statement coverage remains `≥91.0%`.
