# plan: Task 5.9: Disable Critic LLM Reviews

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.1%

Introduce a configuration option `disable_critic` to bypass the LLM-powered review step of `powerword review` (e.g., in git pre-push hooks) to save API tokens and reduce execution time. When disabled, the CLI will still run all local static checks (such as plan relative link validation) and the validation command (e.g., `make all`), ensuring code correctness without invoking expensive LLM calls.

## User Review Required

> [!IMPORTANT]
> **Validation Command Execution**:
> Even when the LLM critic is disabled, we must still run the local validation command (like `make all`). If that command fails, `powerword review` must still exit with an error to prevent committing or pushing broken code. Only the LLM-based critique portion will be skipped.

## Proposed Changes

### Config Subsystem

#### [MODIFY] [config.go](file://../../pkg/config/config.go)
- [x] Add the `DisableCritic bool` field to the `Config` struct.
- [x] In `LoadConfig`, define a default value of `false` for `disable_critic` (so it defaults to active unless explicitly disabled, or we can default to `true` if preferred—we will default to `false` for backward compatibility but disable it in `powerword.example.toml`).
- [x] Bind `disable_critic` to the environment variable `POWERWORD_DISABLE_CRITIC`.

### Review Subsystem

#### [MODIFY] [critic.go](file://../../internal/review/critic.go)
- [x] In `VerifyWorkspace`, check if `cfg.DisableCritic` is true.
- [x] If it is true:
  - Print a message: `"Critic LLM review is disabled in config. Skipping LLM review."`
  - If a validation command is defined (e.g. `"make all"`), execute it directly in the workspace directory.
  - If the validation command fails, print the output and return the compilation/test error.
  - If it succeeds, return `nil` immediately, skipping the MCP process start and the LLM analysis entirely.

#### [MODIFY] [critic_test.go](file://../../internal/review/critic_test.go)
- [x] Add unit tests for `VerifyWorkspace` when `DisableCritic` is true:
  - Verify it runs the validation command and returns `nil` on success.
  - Verify it runs the validation command and returns an error if the command fails.

### Default Repository Configuration

#### [MODIFY] [powerword.example.toml](file://../../powerword.example.toml)
- [x] Add `disable_critic = true` to disable the LLM reviews by default for developers in this repository until a local model (e.g. Ollama) is configured.

---

## Verification Plan

### Automated Tests
- Run `make check-coverage` and verify tests pass with statement coverage at or above **91%**.

### Manual Verification
- Run `powerword review --local` with `disable_critic = true` in `powerword.example.toml` (or by copying it to local `powerword.toml`). Verify that:
  - `make all` runs.
  - No MCP processes are spawned and no LLM API calls are made.
  - It exits with success (`0`).
- Temporarily introduce a syntax error in the codebase and run `powerword review --local`. Verify that it fails and reports the build error.
