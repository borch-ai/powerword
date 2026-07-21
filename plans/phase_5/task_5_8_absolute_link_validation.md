# plan: Task 5.8: Absolute Link Verification & Auto-Fixing

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.1%

Enhance the implementation plan validator to detect and flag absolute filepath links (e.g. `file:///Users/human/code/powerword/...`), and introduce a `--fix` CLI flag to the `powerword review` command to automatically:

1. Convert absolute path links into relative workspace links (preserving the `file://` scheme).
2. Auto-correct mismatched link labels to match the actual file basename.
3. Normalize relative links that lack the `file://` prefix by prepending it.
4. Auto-populate completed plan metadata (`Go Version` from `go.mod` and `Date Completed` to the current date) if the plan is completed but contains placeholders.

To make this automated for developers, we will also add a `fix-plans` target to the repository's `Makefile`.

## User Review Required

> [!IMPORTANT]
> **Validation Strictness**:
> We propose making absolute paths a hard validation error. This means `powerword review` (and therefore pre-push hooks) will fail if a plan file contains absolute links, encouraging developers to keep plans portable.
>
> **CLI `--fix` Flag Integration**:
> To make it easy to fix these errors, we will add a `--fix` flag to the `powerword review` command. Running `powerword review --local --fix` (or `powerword review --issue <id> --fix`) will automatically scan, rewrite the plan files on disk, and then proceed with the normal validation.

## Proposed Changes

### Review Subsystem

#### [MODIFY] [validator.go](file://../../pkg/linter/validator.go)

- Implement `isPathAbsolute(pathStr string) bool` to check if a resolved link path is absolute.
- In `validateLink`, add a check using `isPathAbsolute`:

  ```go
  if isPathAbsolute(pathStr) {
      errs = append(errs, fmt.Sprintf("%s:%d: path %q must be relative, not absolute", planFile, lineNum, pathStr))
  }
  ```

- Implement `FixAbsolutePathsInPlans(workspaceRoot string, cfg *config.Config) (int, error)`:
  - Recursively walks the `plans/` directory to find `task_*.md` files.
  - Parses each file line-by-line.
  - For links under action headers or matching `file://` or relative links:
    - If it's absolute, resolves to the absolute path and computes the relative path using `filepath.Rel(filepath.Dir(planFile), targetAbs)`, formatting it as `file://` + relative path with forward slashes.
    - If it lacks the `file://` prefix, prepends it (e.g., converting `../foo.go` to `file://../foo.go`).
    - Compares the link label with the resolved file's basename; if they mismatch, corrects the label in the brackets to match the basename.
  - For completed metadata (if `Status: Completed` or `Status: completed` is matched in the file):
    - Parses `Go Version` and `Date Completed`.
    - If `Go Version` is missing or a placeholder, reads the Go version from `go.mod` in `workspaceRoot` (or defaults to the runtime version e.g. `1.26.4`) and replaces the line with the actual Go version.
    - If `Date Completed` is missing or a placeholder, formats the current date (`YYYY-MM-DD`) and replaces the line.
  - Writes the modified file back to disk if any changes were made.
  - Returns the total count of modifications made across all plan files.

#### [MODIFY] [validator_test.go](file://../../pkg/linter/validator_test.go)

- Update existing tests (e.g., `TestValidatePlans_CopilotComments`) since they might contain mock absolute paths that would now fail validation.
- Add a new unit test `TestValidatePlans_AbsoluteLinksError` to verify that absolute links are correctly caught and flagged.
- Add a unit test `TestFixAbsolutePathsInPlans` to verify that:
  - Absolute links are successfully converted to relative ones.
  - Mismatched labels are corrected.
  - `file://` prefix is added to naked relative links.
  - Go Version and Date Completed metadata are successfully auto-populated from `go.mod` and today's date when the plan status is `Completed`.

### Command Line Interface

#### [MODIFY] [review.go](file://../../cmd/powerword/review.go)

- Add a `--fix` boolean flag to the `review` subcommand:

  ```go
  cmd.Flags().BoolVar(&fixPlans, "fix", false, "Automatically convert absolute file path links in plan files to relative paths")
  ```

- In the `RunE` block, if `fixPlans` is true, invoke `review.FixAbsolutePathsInPlans(".", cfg)` before verifying the workspace. Log the number of files/links fixed.

### Build and Tooling Configuration

#### [MODIFY] [Makefile](file://../../Makefile)

- Add the `.PHONY` target `fix-plans` and its recipe:

  ```makefile
  fix-plans:
   $(GOCMD) run ./cmd/powerword review --local --fix
  ```

---

## Verification Plan

### Automated Tests

- Run unit tests: `go test -v ./internal/review/...`
- Ensure that unit test coverage continues to meet the strict **91% coverage requirement** (`make check-coverage`).

### Manual Verification

- Create a test plan file containing an absolute path, e.g., `[validator.go](file://../../internal/review/validator.go)`.
- Run `powerword review --local` and verify it exits with a non-zero status and reports errors.
- Run `make fix-plans` and verify it automatically runs the tool via `go run` and corrects the plan file to use `[validator.go](file://../../internal/review/validator.go)`.
