# Task 4.2: Headless Pipelines & Automation

Implement non-interactive input/output mechanisms for running Powerword inside CI/CD pipelines, automated cron-jobs, or unix piping chains. Enable structured formatting options (JSON output) and disable all interactive terminal prompts.

## User Review Required

> [!WARNING]
> Running in headless mode implies absolute automation. Tool execution guardrails must automatically fail the execution if a tool requires interactive verification and no override flags are passed.

## Proposed Changes

### Headless Execution Modes

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Adds CLI flags:
  - `--headless` (disables interactive confirmations, defaults to strict safety rejection of unsafe commands).
  - `--json` (encapsulates stdout inside a structured JSON payload: response text, token usage stats, list of executed tools, execution status).
  - `--accept-all` (allows bypass of interactive prompts for scripting convenience - to be used with extreme caution).

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Detects the headless flag.
- Reading standard input: if no prompt is passed as arguments, reads input directly from `os.Stdin`.
- Modifies output printing to skip real-time streaming to terminal stdout if structured JSON format is requested. Instead, collects full buffers and outputs the final serialized JSON object.

---

## Verification Plan

### Automated Tests
- Test streaming standard input feeds to the loop processor.
- Test structured JSON format validation to ensure output schemas comply with stable API schemas.
- Test safety termination behavior in headless mode when a destructive action is requested without `--accept-all`.

### Manual Verification
- Pipe command contents to Powerword: `$ cat README.md | powerword --headless "summarize this file" --json`. Ensure the output is valid JSON on stdout, with exit code `0`.
