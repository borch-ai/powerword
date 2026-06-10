# plan: Task 5.1: Structured Issue Templates & Local Critic Integration

**Status:** Completed (Issue #24)


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
  - ## Phase 1: Local Validations

    1.  **Pre-requisite Check**:
        *   The command first determines if the workspace defines a `Makefile`. If `Makefile` is present, it will run `make all` with a timeout of 3 minutes.
        *   Output is captured. If it fails, the process exits and writes to `.powerword-critic.md` locally.

    2.  **Diff Extraction**:
        *   Instead of blindly pushing code, the agent extracts the diff using the local `pw-mcp-git` MCP server.
        *   **Update**: Modified to call both `git_diff_commits` (diff between `main` and `HEAD`) and `git_diff` (uncommitted working tree changes).
        *   This provides a comprehensive view of all changes related to the implementation plan, preventing the critic from falsely accepting a clean working tree that lacks unpushed commits.

    ## Phase 2: AI Critic Invocationed.
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

### Final Implementation Details (Post-Critic Review)
- **Timeouts**: Added a 3-minute `context.WithTimeout` to the `VerifyWorkspace` step so a hung local build doesn't indefinitely block pushing code.
- **Payload Sanitization**: Cleaned up the `parseIssueBody` function to strip out generic empty form placeholders (`_No response_`) provided by GitHub Issue Forms.
- **Robust Branch Regex**: Tightened up the ID extraction logic in `.git/hooks/pre-push` to accurately isolate branch IDs.
- **Issue Form Locking**: Added `.github/ISSUE_TEMPLATE/config.yml` to disable blank issues and enforce structured templates for all new issues.
- **Coverage Mock Logic**: Expanded `internal/review/critic_test.go` and refactored file existence check strategies for `Makefile` and `pw-mcp-git` to allow them to be tested safely across diverse `go test` environments without relying on the actual repo structure. Tests now inject invalid Git MCP commands to deterministically trigger failures.
- **Nil Guards & Fallbacks**: Added nil checks to `VerifyWorkspace`, enforced string match validations (e.g. `strings.HasSuffix`), fallback logic for `critic_model`, and URL normalization for OpenAI-compatible base URLs.
- **Documentation**: Clarified in `GEMINI.md` that `.powerword-critic.md` is transient and cleaned up upon successful execution or when no changes are found.

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
