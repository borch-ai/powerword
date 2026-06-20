# Plan: Task 6.26: Centralized Pull Request Issue Linker Subcommand & GitHub Adaptations

**Status:** Proposed (Issue #26)
**Go Version:** 1.26
**Unit Test Coverage Target:** 91%

This plan introduces a native Go subcommand `powerword link-issue` to scan workspace plans for issue references and auto-link them in the pull request description. It also adopts and adapts the `.github` configurations from `aeolian` (Copilot rules, pull request template, dependabot.yml, and automated workflows) for `powerword`.

---

## User Review Required

> [!IMPORTANT]
> **GitHub CLI Dependency (`gh`)**:
> The `powerword link-issue` command will run shell commands against `gh` CLI (e.g. `gh pr view` and `gh pr edit`). This requires the CI runner or the local user to have `gh` installed and authenticated (or supply `GITHUB_TOKEN` in the environment). This matches the dependency model of `powerword review` (Task 5.1).

---

## Proposed Changes

### CLI Subcommand

#### [NEW] [link_issue.go](file://../../cmd/powerword/link_issue.go)
*   Define the `link-issue` command with flags:
    *   `--pr` (string): The target Pull Request number.
    *   `--base` (string): The base ref/branch name (defaulting to `main`).
*   Retrieve values from environment variables (`PR_NUMBER`, `BASE_REF`) if flags are omitted.
*   If neither flags nor environment variables are found, execute `gh pr view --json number,baseRefName` to auto-detect the context.

#### [MODIFY] [main.go](file://../../cmd/powerword/main.go)
*   Register `newLinkIssueCmd()` inside `main()`.

---

### Core Review/Git Component

#### [NEW] [link_issue.go](file://../../internal/review/link_issue.go)
*   Implement `LinkTaskIssue(ctx context.Context, prNumber, baseRef string)`:
    1.  Fetch the base branch using `exec.CommandContext("git", "fetch", "origin", baseRef, "--depth=1")` (fall back to full fetch if failing).
    2.  Obtain the list of changed files via `git diff --name-only origin/<baseRef>...HEAD`.
    3.  Scan any modified file matching `^plans/.*\.md$` for references of `Issue\s+#(\d+)`.
    4.  Fetch the current PR body text using `gh pr view <prNumber> --json body`.
    5.  Scan the body text using a regex for standard GitHub closing references (e.g. `(?:closes|resolves|fixes)\s+#<id>\b`).
    6.  For any missing references, build the updated PR description body, appending:
        ```markdown
        <!-- Auto-linked via CI plan checker -->
        Closes #<id>
        ```
    7.  Save the updated body to a temporary file and run `gh pr edit <prNumber> --body-file <tempFile>` to update the PR.

#### [NEW] [link_issue_test.go](file://../../internal/review/link_issue_test.go)
*   Write table-driven unit tests verifying the regex matching, missing issue detection, and parser helper logic using mocks for `exec.CommandContext`.

---

### GitHub Configurations & Workflows

#### [NEW] [copilot-code-review-instructions.md](file://../../.github/copilot-code-review-instructions.md)
*   Define instructions tailored to `powerword`'s unique architectural patterns:
    *   LLM Provider decoupling interfaces.
    *   No live network requests in unit tests.
    *   Config bindings via Cobra/Viper using `POWERWORD_` prefix.
    *   Strict 91% unit test coverage requirement.

#### [NEW] [dependabot.yml](file://../../.github/dependabot.yml)
*   Set up weekly dependabot checks for `gomod` and `github-actions`.

#### [NEW] [pull_request_template.md](file://../../.github/pull_request_template.md)
*   Port the template structure (Overview, Changes, Verification, Related Issues) from Aeolian.

#### [NEW] [link-task-issue.yml](file://../../.github/workflows/link-task-issue.yml)
*   GitHub Action running on pull request events (`opened`, `edited`, `reopened`, `synchronize`).
*   Runs `powerword link-issue` using the compiled binary.

---

## Verification Plan

### Automated Tests
*   Run unit tests: `go test ./internal/review/...`
*   Ensure unit test coverage across the modified files meets the 91% threshold.

### Manual Verification
*   Simulate the issue linking process locally by creating mock plan files, running a test branch, and running `powerword link-issue --pr <mock_pr_number> --base main`.
