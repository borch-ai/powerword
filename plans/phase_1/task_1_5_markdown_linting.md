# plan: Task 1.5: Markdown Linting & Implementation Plan Validation

**Status:** Completed (Issue #40)
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%

Establish continuous integration gates to ensure all documentation files (.md) follow strict Markdown styling rules, and verify that all files under `plans/` conform to the standard structure of the core template.

## User Review Required

> [!NOTE]
> **Go-Native Solution**: We chose a Go-native markdown linter and plan validator rather than using external Node.js/NPM dependencies (like `markdownlint-cli`). This keeps the repository lightweight and self-contained for Go developers.

## Actual Choices & Configurations

- **Go Version Used**: Go 1.26.4
- **Formatting and Linting Tools Configured**:
  - A Go-native markdown linter implemented under [linter.go](file:///Users/human/code/powerword/internal/linter/linter.go) and unit-tested in [linter_test.go](file:///Users/human/code/powerword/internal/linter/linter_test.go).
  - A plan template validator implemented under [linter.go](file:///Users/human/code/powerword/internal/linter/linter.go).
  - Multi-file scripts runner organized under [main.go](file:///Users/human/code/powerword/scripts/lint_markdown/main.go) and [main.go](file:///Users/human/code/powerword/scripts/lint_plans/main.go).
  - Makefile targets integrated into the default validation pipeline (`make all` and `make markdown-lint`).
  - GitHub Actions CI workflow updated to run `make markdown-lint` as part of `ci.yml`.

## Proposed Changes

### Configuration & Scripters

#### [NEW] [linter.go](file:///Users/human/code/powerword/internal/linter/linter.go)
- Core Go-native logic for checking basic Markdown formatting:
  - `MD024` (Duplicate headers): Enabled to prevent duplicate header strings within the same file.
  - `MD029` (Ordered list prefix): Standardized to use `1.` style prefixes or sequential prefixes starting from 1.
  - Skips text within code blocks to prevent false positives.
- Core Go-native logic for checking plan files under `plans/` for required template headings.

#### [NEW] [linter_test.go](file:///Users/human/code/powerword/internal/linter/linter_test.go)
- Comprehensive unit tests covering header duplicates, nested lists, list resets, style violations, and plan templates.
- Enforces strict coverage threshold.

#### [NEW] [main.go](file:///Users/human/code/powerword/scripts/lint_markdown/main.go) <!-- markdown -->
- Walks the project directory and checks all markdown files with `linter.LintMarkdown`.

#### [NEW] [main.go](file:///Users/human/code/powerword/scripts/lint_plans/main.go)
- Scans `plans/*.md` (excluding `TEMPLATE.md`) and validates headings with `linter.LintPlan`.

#### [MODIFY] [Makefile](file:///Users/human/code/powerword/Makefile)
- Adds a `markdown-lint` target which runs both script commands.
- Integrates `markdown-lint` into the standard `make all` validation target pipeline.

#### [MODIFY] [ci.yml](file:///Users/human/code/powerword/.github/workflows/ci.yml)
- Adds a build step to run `make markdown-lint` to check PR validation.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/linter/...` to execute the new linting logic tests.
- Run `make check-coverage` to verify the coverage.

### Manual Verification
- Run `make markdown-lint` on the current repository. Verify it checks all markdown files successfully and passes with no style issues.
- Temporarily rename a heading in one of our existing plans and check if `make markdown-lint` catches the issue and returns exit code 1.
