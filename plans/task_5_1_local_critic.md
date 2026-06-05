# Task 5.1: Structured Issue Templates & Local Critic Integration

Define GitHub YAML Issue Forms to enforce structured planning schemas, and implement a `powerword review` sub-command to parse active issue fields and verify local Git diffs against plan goals.

## User Review Required

> [!NOTE]
> **Issue Template Configuration**:
> We will configure a new GitHub YAML issue form template `.github/ISSUE_TEMPLATE/implementation_plan.yml` defining strict input fields: Goal Description, User Review Required, Proposed Changes, and Verification Plan.

> [!IMPORTANT]
> **Local LLM Integration**:
> We will configure a fallback local LLM target (e.g. Ollama or a local Llama-3 endpoint) specifically for pre-flight critics to avoid consuming commercial API tokens for rapid local checks.

## Proposed Changes

### GitHub Issue Templates

#### [NEW] [implementation_plan.yml](file:///Users/human/code/powerword/.github/ISSUE_TEMPLATE/implementation_plan.yml)
- Define a structured YAML GitHub Issue Form for submitting implementation plans, enforcing sections for proposed changes, lints, and verification steps.

### Review Component

#### [NEW] [critic.go](file:///Users/human/code/powerword/internal/review/critic.go)
- Implement issue retrieval using the `gh` CLI wrapper or direct GitHub API calls.
- Parse the fetched issue's structured fields (Goal, Proposed Changes, Verification steps) into a target plan struct.
- Fetch staging or HEAD Git diffs and use LLM to check if changes match the plan constraints and codebase rules.

### Command Configuration

#### [MODIFY] [config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Add configurations for `Review` (defining local LLM model provider, prompt templates, and active GitHub repository details).

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Create and register `powerword review` command with flag support (`--issue <id>` and `--local`).

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/review/...`
- Unit tests verifying:
  - Markdown/YAML issue payload parser (mapping raw text fields to struct properties).
  - Mock git diff parsing.
  - Critic prompt generation using templates.

### Manual Verification
- Create a mockup implementation plan issue on a test repo.
- Run `powerword review --issue <id>` and verify that the critic correctly fetches the plan, reads your local edits, and outputs suggestions.

