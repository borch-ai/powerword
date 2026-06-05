# Task 5.1: Local Critic & Code-Review Simulator (Pre-Flight Checks)

Implement a specialized sub-command `powerword review --local` to analyze local Git diffs against structured codebase rules before pushing to remote branches.

## User Review Required

> [!NOTE]
> **Codebase Rule Discovery**:
> The simulator will discover rules dynamically by looking for a local `.rules/` or `.github/instructions/` directory in the target repository, making the review criteria customizable per codebase.

> [!IMPORTANT]
> **Local LLM Integration**:
> We will configure a fallback local LLM target (e.g. Ollama or a local Llama-3 endpoint) specifically for pre-flight critics to avoid consuming commercial API tokens for rapid local checks.

## Proposed Changes

### Review Component

#### [NEW] [critic.go](file:///Users/human/code/powerword/internal/review/critic.go)
- Implement diff parsing logic using raw Git commands (`git diff` from staging/HEAD).
- Load local instruction sets (e.g. from `GEMINI.md`, rules directory).
- Call LLM client to parse the diff against the instructions and print suggestions in a clean CLI format.

### Command Configuration

#### [MODIFY] [config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Add configurations for `Review` (defining local LLM model provider, prompt targets, and instructions directory).

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Create and register the `reviewCmd` under Cobra root.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/review/...`
- Unit tests verifying:
  - Mock git diff parsing.
  - Critic prompt generation using templates.
  - Mock LLM response parsing and layout formatting.

### Manual Verification
- Create a dummy file with a violation (e.g., unused variable or print statement).
- Run `powerword review --local` and verify that the critic correctly flags the violation and recommends a fix.
