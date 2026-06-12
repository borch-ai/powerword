# plan: Task 4.2: Headless Pipelines & Automation

**Status:** Completed (Issue #54)
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%


Implement non-interactive input/output mechanisms for running Powerword inside CI/CD pipelines, automated cron-jobs, or unix piping chains. Enable structured formatting options (JSON output) and disable all interactive terminal prompts.

## User Review Required

> [!WARNING]
> Running in headless mode implies absolute automation. Tool execution guardrails must automatically fail the execution if a tool requires interactive verification and no override flags are passed.

## Proposed Changes

### Headless Execution Modes

#### [MODIFY] [root.go](file:///Users/human/code/powerword/pkg/config/root.go)
- Adds CLI flags:
  - `--headless` (disables interactive confirmations, defaults to strict safety rejection of unsafe commands).
  - `--json` (encapsulates stdout inside a structured JSON payload: response text, list of executed tools, execution status). *Note: Token usage stats will be added to this payload later in Task 4.3.*
  - `--accept-all` (allows bypass of interactive prompts for scripting convenience - to be used with extreme caution).

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Detects the headless flag.
- Reading standard input: if no prompt is passed as arguments, reads input directly from `os.Stdin`. If a prompt argument is passed, it takes precedence.
- Output routing: When `--json` is enabled, all normal CLI logs, warnings, and stream outputs are forced to `stderr`. The final `JSONPayload` is the only text printed to `stdout` to ensure pipeline compatibility.

---

## Verification Plan

### Automated Tests
- Test streaming standard input feeds to the loop processor.
- Test structured JSON format validation to ensure output schemas comply with stable API schemas.
- Test safety termination behavior in headless mode when a destructive action is requested without `--accept-all`.

### Manual Verification
- Pipe command contents to Powerword: `$ cat README.md | powerword --headless "summarize this file" --json`. Ensure the output is valid JSON on stdout, with exit code `0`.

---

## Final Status Updates
- **Actual Configurations:** The `--headless` and `--json` configurations correctly map to Viper in `internal/config/config.go`.
- **Testing Updates:** Extensive testing has been added to `internal/loop/loop_test.go` and `internal/config/root_test.go`, including `os.Pipe` output inspection, ensuring we retain exactly 91.00% test coverage.
- **Go Version:** Go 1.23+ is verified as the standard across this task and all components.
