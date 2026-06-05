# Task 2.4: Interactive Permission & Consent Manager

Implement an interactive security barrier in the CLI core routing loop. Intercept tool calling commands requested by the LLM and require manual yes/no terminal confirmations before executing unsafe commands (like write operations, git commits, or shell invocations).

## User Review Required

> [!CAUTION]
> Prompt Bypass: In headless/non-interactive workflows (e.g. CI/CD runs), interactive prompts are impossible. The permission manager must fail-closed by default unless explicitly bypassed with a `--accept-all` CLI override flag.

## Proposed Changes

### Interceptors & Terminal Guards

#### [NEW] [guard.go](file:///Users/human/code/powerword/internal/loop/guard.go)
- Defines security profiles:
  - `ReadOnly` (allow reads, block modifications/scripts).
  - `Interactive` (default; prompt for modifying actions).
  - `Bypass` (trust all actions).
- Implements:
  - `Authorize(toolName string, args map[string]interface{}) (bool, error)`
  - Renders styled interactive terminal prompts (using color prompts and raw stdin reads).

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Intercepts tool execution requests within the reasoning loop.
- Calls `guard.Authorize(...)`. If declined, returns a permission error message back to the LLM (so the LLM can try an alternative approach).

---

## Verification Plan

### Automated Tests
- Test permission interceptor in mock non-interactive terminal configurations to verify executions fail-closed securely.
- Mock consent inputs (pressing `y` vs `n` in stdin reader) and check corresponding routing returns.

### Manual Verification
- Ask the model: `"Delete the file ./temp.txt"`. Verify that the CLI intercepts the FS plugin `delete_file` call and prompts in the terminal: `[?] Allow tool filesystem.delete_file? (y/N)`. Declining should write a standard response to the model.
