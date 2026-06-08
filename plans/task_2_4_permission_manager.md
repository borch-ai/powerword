# Task 2.4: Interactive Permission & Consent Manager

**Status:** Completed
**Go Version:** 1.26.4

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
- **Interactive Mode (Mutating Action):** Ask the model: `"Write some text to the file ./temp.txt"`. Verify that the CLI intercepts the FS plugin `write_file` call and prompts in the terminal: `[?] Allow tool filesystem_write_file? (y/N)`. Declining should write a standard error response to the model, allowing it to realize it couldn't write the file.
- **Interactive Mode (Read-Only Action):** Ask the model to `"List the files in the current directory"`. Verify that the tool executes immediately without prompting the user.
- **Bypass Mode:** Run the CLI with the `--accept-all` flag and ask it to `"Create a file named temp2.txt"`. Verify that the file is created automatically without any interactive prompt appearing.
- **EOF/Headless Safety:** Run the CLI without `--accept-all` and pipe input into it (e.g., `echo "Write to temp.txt" | powerword`). Verify that it fails-closed upon reaching EOF when attempting to prompt for a mutating action.
