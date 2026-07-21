# plan: Task 6.10: Headless JSON Envelopes & PR Review Mode

**Status:** Open (Issue #TBD)

This task creates structured JSON envelopes for headless runner outputs and implements automated Pull Request review modes to comment on changes directly via the GitHub API.

## User Review Required

> [!WARNING]
> Running code as a PR review agent requires a valid GitHub Access Token (`GITHUB_TOKEN`) with read/write access to PR comments.

## Proposed Changes

### Headless Engine

#### [MODIFY] [main.go](file://../../cmd/powerword/main.go)

- [ ] Add CLI option flags for `--json-envelope` and `--pr-review-mode`.

#### [MODIFY] [loop.go](file://../../internal/loop/loop.go)

- [ ] Implement wrapper schemas to structure all pipeline outputs in JSON formats.
- [ ] Integrate GitHub API clients to post comment streams matching linter rules back to corresponding commits.

---

## Verification Plan

### Automated Tests

- [ ] Run `go test ./internal/loop/...` testing parsing of JSON outputs and GitHub client mock structures.

### Manual Verification

- [ ] Run in headless mode checking that output adheres to JSON format specifications.
