# plan: Task 6.3: Pause & Resume Session States

**Status:** Open (Issue #TBD)

This task implements session pause and resume mechanisms, enabling long-running agent loops to serialise their state (context history, task list, tool outputs) to a file and pause for manual human fixes or review before resuming.

## User Review Required

> [!NOTE]
> Serializing LLM session state requires careful preservation of system prompt updates, intermediate tool call payloads, and the exact state of custom task tracking items (`task.md`).

## Proposed Changes

### Core Loop and Sessions

#### [MODIFY] [session.go](file:///Users/human/code/powerword/internal/loop/session.go)
- [ ] Add serialization methods to dump current session state variables (history, active plans) to a JSON session file.
- [ ] Implement a command loader to parse and reconstruct session state variables on initialization.

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- [ ] Support `--resume <session_id>` CLI flag to bypass initial greeting/system prompts and restore previous loop state.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/loop/...` verifying state serialization and deserialization compatibility.

### Manual Verification
- [ ] Start a session, run a tool, interrupt execution, and use the resume command to verify the execution history is preserved.
