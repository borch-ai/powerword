# Task 1.5: Markdown Linting & Implementation Plan Validation

Establish continuous integration gates to ensure all documentation files (.md) follow strict Markdown styling rules, and verify that all files under `plans/` conform to the standard structure of the core template.

## User Review Required

> [!NOTE]
> Markdown Tooling: We will use `markdownlint-cli` (an npm module) or a lightweight Go equivalent to perform local markdown style checks during build stages. We will check for the presence of node/npm on the system.

## Proposed Changes

### Configuration & Scripters

#### [NEW] [.markdownlint.json](file:///Users/human/code/powerword/.markdownlint.json)
- Configures rules for `markdownlint-cli`:
  - `MD013` (Line length): Disabled or customized to allow long lines (e.g. 120 chars or infinite for headings/paragraphs).
  - `MD024` (Duplicate headers): Enabled to prevent duplicate header strings.
  - `MD029` (Ordered list prefix): Standardized to use `1.` style prefixes.

#### [NEW] [lint_plans.go](file:///Users/human/code/powerword/scripts/lint_plans.go)
- A Go script that scans the `plans/` directory.
- For each `.md` file (except `TEMPLATE.md`), checks if it contains the required template headings:
  - `# [Goal Description]` (or similar top-level header)
  - `## User Review Required`
  - `## Proposed Changes`
  - `## Verification Plan`
- Fails with a descriptive error specifying which headings are missing if validation fails.

#### [MODIFY] [Makefile](file:///Users/human/code/powerword/Makefile)
- Adds a `markdown-lint` target:
  - Checks if `markdownlint` is installed. If not, warns or installs locally.
  - Runs `markdownlint **/*.md`.
  - Runs `go run scripts/lint_plans.go`.
- Integrates `markdown-lint` into the standard `make all` validation target pipeline.

---

## Verification Plan

### Automated Tests
- Test `lint_plans.go` against a mock directory containing:
  - A correctly formatted mock plan.
  - A plan missing the "Proposed Changes" heading (must fail validation).

### Manual Verification
- Run `make markdown-lint` on the current repository. Verify it checks all markdown files successfully and passes with no style issues.
- Temporarily rename a heading in one of our existing plans and check if `make markdown-lint` catches the issue and returns exit code 1.
