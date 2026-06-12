# plan: Task 5.6: Test Coverage Check Command

**Status:** Completed
**Go Version:** Go 1.26.4
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91.1%

Add a project-agnostic `powerword check-coverage` subcommand to the Powerword CLI. This subcommand parses standard Go test coverage profile outputs, computes total statement coverage, and compares it against a configured minimum threshold percentage. This enables workspaces to enforce unit test coverage gates without maintaining custom checking scripts locally.

## User Review Required

> [!IMPORTANT]
> **Subcommand Interface**:
> The subcommand will be registered as:
> `powerword check-coverage <threshold> [profile_path]`
> If the parsed coverage percentage is lower than the threshold, the command will print diagnostic details and exit with status code `1`, aborting the push or validation flow.

---

## Proposed Changes

### Command Line Interface

#### [NEW] [check_coverage.go](file://../../cmd/powerword/check_coverage.go)
- Define and register the `check-coverage` Cobra subcommand under the root command.
- Set arguments constraints (requires minimum 1 arg representing threshold, optionally accepts profile path).
- Handler logic:
  1. Parse `<threshold>` argument to float64.
  2. Parse `[profile_path]` (defaults to `"coverage.out"`).
  3. Invoke the coverage verification routine `review.VerifyCoverage(threshold, profilePath)`.

### Review Subsystem

#### [NEW] [coverage.go](file://../../internal/review/coverage.go)
- Implement `VerifyCoverage(threshold float64, profilePath string) error`:
  1. **Clean NULL bytes:** Read and sanitize the target coverage profile file, stripping any null bytes or incomplete lines to prevent parser syntax errors.
  2. **Run go tool cover:** Execute `go tool cover -func=<profilePath>` inside the workspace context.
  3. **Parse covered percentage:** Read the stdout, capture the total coverage statement line (`total: (statements) X.Y%`), and parse the float value `X.Y`.
  4. **Compare to threshold:** If parsed value is less than `<threshold>`, return an error indicating the coverage deficit.

---

## Verification Plan

### Automated Tests
- Create `internal/review/coverage_test.go` verifying:
  - Sanitization of coverage files containing null bytes.
  - Correct parsing of `go tool cover` output blocks.
  - Proper error return when parsed coverage is under the threshold.
  - Successful validation when coverage matches or exceeds the threshold.
- Command: `go test -v ./internal/review/...`
- Ensure that unit test coverage across the modified packages meets or exceeds the **91% threshold**.

### Manual Verification
- In the Pithos or Powerword repository, run `powerword check-coverage 91.0 coverage.out`.
- Verify it correctly parses and prints output, exiting with code `0`.
- Run `powerword check-coverage 99.9 coverage.out` and verify it fails, printing the failure diagnostic and exiting with code `1`.
