# plan: Task 5.5: Plan Conformance & Validation Integration

**Status:** Completed
**Go Version:** Go 1.26.4
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91.1%

Integrate plan template and relative link conformance checks directly into the `powerword review` command. This centralizes the plan checking logic inside Powerword, allowing any project to validate its local implementation plans as a fail-fast pre-review step without maintaining local validation scripts.

## User Review Required

> [!IMPORTANT]
> **Plan Validator Integration**:
> The check will be executed at the very beginning of the `powerword review` command (both local and issue-based reviews). If any validation errors are found, the command will exit immediately with an error, avoiding compile commands and LLM network requests.
>
> [!NOTE]
> **Template Resolution Order**:
> To make plan templates universally shareable, we will resolve templates using the following fallback resolution order:
>
> 1. Workspace-specific: `plans/TEMPLATE.md` in the current repository.
> 2. Global Config: Configured path `plan_template` in `powerword.toml`.
> 3. Embedded Default: Generic default template embedded directly within the `powerword` binary.

---

## Proposed Changes

### Configuration

#### [MODIFY] [config.go](file://../../pkg/config/config.go)

- Add `PlanTemplate` field to the `Config` structure (`plan_template` mapstructure tag).
- Bind it to environment variable `POWERWORD_PLAN_TEMPLATE` and default settings.

### Review Subsystem

#### [NEW] [default_template.md](file://../../pkg/linter/default_template.md)

- Define a generic default implementation plan template to embed.

#### [NEW] [validator.go](file://../../pkg/linter/validator.go)

- Use `//go:embed default_template.md` to embed the generic fallback template.
- Implement `ValidatePlans(workspaceRoot string, cfg *config.Config) error` which:
  1. Resolves the active template text using the resolution order (workspace file -> global config path -> embedded fallback).
  2. Scans for all plans matching `plans/task_*.md` in `workspaceRoot`.
  3. Validates each plan has headers: `# plan: Task ...`, `## Proposed Changes`, and `## Verification Plan`.
  4. Parses plan status. If the plan status is `Completed`, enforces presence of metadata fields:
     - `**Go-Version:**`
     - `**Date-Completed:**`
     - `**Unit-Test-Coverage:**`
  5. Extracts markdown links in the format `\<label\>(\<url\>)`. If it is an action header (contains `[NEW]`, `[MODIFY]`, or `[DELETE]`) or starts with `file://`, validates that:
     - Path is inside the workspace root (resolving relative paths relative to the `plans/` folder).
     - The link label matches the actual file basename.
     - Modified files (`[MODIFY]`) exist on disk.
     - Completed new files (`[NEW]` when `Status: Completed`) exist on disk.
- If any validation errors are found, aggregate all errors across files and return a structured validation error.

#### [MODIFY] [critic.go](file://../../internal/review/critic.go)

- At the start of `VerifyWorkspace(...)`, run `ValidatePlans(".", cfg)`.
- Return the conformance errors immediately on failure to enforce a fail-fast quality gate.

---

## Verification Plan

### Automated Tests

- Create `internal/review/validator_test.go` verifying:
  - Falling back to embedded template when no local/global template exists.
  - Using global config template.
  - Local template override.
  - Structural heading and relative/absolute link checks.
  - Required completed plan metadata validation.
- Command: `go test -v ./internal/review/...`
- Verify that unit test coverage across modified packages meets or exceeds the **91% threshold**.

### Manual Verification

- Define a global template path in `~/.config/powerword/config.toml`. Delete the local `plans/TEMPLATE.md` in Pithos.
- Introduce a plan validation failure in a Pithos plan.
- Run `powerword review --local` inside Pithos and verify that the global template is loaded and the validation correctly fails.
