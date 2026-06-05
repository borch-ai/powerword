# Task 5.1: Structured Issue Templates & Local Critic Integration

Configure a structured GitHub Issue Form template for project plans, and implement the local client engine in `powerword` to parse these issues and verify local workspace diffs against them before pushing.

## User Review Required

> [!NOTE]
> **Issue Schema Format**:
> We will define a GitHub Issue Form template in `.github/ISSUE_TEMPLATE/implementation-plan.yml`. This template uses structured fields to enforce key sections: `Goal`, `Proposed Changes`, and `Verification Plan`.

> [!IMPORTANT]
> **GitHub CLI (`gh`) Dependency**:
> The local client will rely on the authenticated `gh` CLI tool (or direct HTTP client using `GITHUB_TOKEN`) to fetch and parse issue data as structured JSON.

---

## Proposed Changes

### GitHub Issue Templates

#### [NEW] [implementation-plan.yml](../.github/ISSUE_TEMPLATE/implementation-plan.yml)
- Define a structured YAML-based GitHub Issue Form. This ensures that when a new plan is created as an issue, it has distinct, parseable sections:
  ```yaml
  name: Implementation Plan
  description: Design plan for a new task
  body:
    - type: textarea
      id: goal
      attributes:
        label: Goal
    - type: textarea
      id: changes
      attributes:
        label: Proposed Changes
    - type: textarea
      id: verification
      attributes:
        label: Verification Plan
  ```

### Review & Verification Component

#### [NEW] [critic.go](../internal/review/critic.go)
- Implement `LoadIssuePlan(issueID int)` to execute `gh issue view <id> --json body,comments` and parse the YAML/Markdown sections into a structured Go struct.
- Implement `VerifyWorkspace(ctx context.Context, plan *Plan)`:
  - Run the local validation suite (`make all`, which runs `make lint`, `make test`, `make vuln`, `make markdown-lint`).
  - Extract the current git diff: `git diff HEAD`.
  - Pass the plan, the local validation output, and the git diff to a local LLM client (configured via Ollama or a cost-effective API endpoint) to verify that all proposed changes have been implemented and verified.
  - Return a detailed report of any omissions, bugs, or untested files.

### Git Hooks

#### [NEW] [pre-push](../scripts/git-hooks/pre-push)
- Shell script acting as a Git hook. It automatically executes `make all` and/or `powerword review --local` prior to any `git push`.
- Aborts the push if any checks fail.

#### [MODIFY] [Makefile](../Makefile)
- Add an `install-hooks` target to copy `scripts/git-hooks/pre-push` into `.git/hooks/pre-push` and set executable permissions.

### Command Configuration

#### [MODIFY] [config.go](../internal/config/config.go)
- Add configuration parameters for local critic (e.g. local LLM endpoint, model name, and path to local rules).

#### [MODIFY] [root.go](../internal/config/root.go)
- Create and register `powerword review` command with flags:
  - `--issue`: GitHub issue ID containing the active plan.
  - `--local`: Run local validation and ruleset verification only (without fetching a remote issue).

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/review/...`
- Unit tests verifying:
  - JSON/YAML parser for GitHub issue body content.
  - Mock git diff extractor and parser.
  - Critic prompt building and local LLM response handling.

### Manual Verification
- Create a test issue using the new template.
- Make a change in the workspace that violates a project rule or is missing from the plan.
- Run `powerword review --issue <id>` and verify that the local critic flags the discrepancy and outputs recommendations.
