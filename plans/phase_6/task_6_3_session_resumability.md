# plan: Task 6.3: Pause & Resume Session States

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.1% (meets strict >91% threshold requirement)

This task implements session pause and resume mechanisms, enabling long-running agent loops to serialise their state (context history, task list, tool outputs) to a file and pause for manual human fixes or review before resuming.

## User Review Required

> [!NOTE]
> Serializing LLM session state requires careful preservation of system prompt updates, intermediate tool call payloads, and the exact state of custom task tracking items (`task.md`).

---

## Proposed Changes

### Core Loop and Sessions

#### [MODIFY] [session.go](file:///Users/human/code/powerword/internal/loop/session.go)
- [x] Add serialization methods to dump current session state variables (history, active plans) to a JSON session file.
- [x] Implement a command loader to parse and reconstruct session state variables on initialization.

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- [x] Support `--resume <session_id>` CLI flag to bypass initial greeting/system prompts and restore previous loop state.
- [x] Integrate session saving on pause with clean exit.
- [x] Address PR reviews: ensure JSON output uses stderr on pause, and reject prompt+resume combined flags.

#### [MODIFY] [guard.go](file:///Users/human/code/powerword/internal/loop/guard.go)
- [x] Add a pause response option `p`/`pause` to the interactive prompt.
- [x] Define a sentinel `ErrSessionPaused` to notify the loop to serialize and halt.

### Configuration Layer

#### [MODIFY] [config.go](file:///Users/human/code/powerword/pkg/config/config.go)
- [x] Add configuration structures to track the `Resume` session target.

#### [MODIFY] [root.go](file:///Users/human/code/powerword/pkg/config/root.go)
- [x] Expose `--resume` option in Cobra CLI configuration.
- [x] Check environment variables (`POWERWORD_RESUME`) for resume targets.

### CI Integration

#### [MODIFY] [ci.yml](file:///Users/human/code/powerword/.github/workflows/ci.yml)
- [x] Simplify pushes to main trigger configurations.

---

## Verification Plan

### Automated Tests
- [x] Run `make test` and `make check-coverage` to ensure unit test coverage meets the >91.0% requirement.
- [x] Added unit tests in `guard_test.go` and `loop_test.go`.
- [x] Added integration/system tests in `cli_integration_test.go` covering full pause/resume E2E flows, environment variables configuration, and user rejection scenarios.

### Manual Verification
- [x] Start a session, run a tool, interrupt execution, and use the resume command to verify the execution history is preserved.

