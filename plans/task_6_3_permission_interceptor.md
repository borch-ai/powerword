# plan: Task 6.3: Asynchronous Permission Interceptor API

**Status:** Open (Issue #30)

Refactor Powerword's interactive command-line permission gate to support asynchronous remote callbacks. When the agent attempts a mutating tool call (e.g., modifying a file or running a shell command), it will pause execution, send an approval request to the mobile device via the WebRTC data channel, and resume only when authorized.

## User Review Required

> [!CAUTION]
> A timeout is implemented (5 minutes) to ensure goroutines do not leak if a response is never received from the remote mobile dashboard. The permission gate fails closed.

## Proposed Changes

### Security Guard Broker
#### [MODIFY] [internal/loop/guard.go](file:///Users/human/code/powerword/internal/loop/guard.go)
- Refactor `Authorize(toolName string, args map[string]interface{}) (bool, error)`.
- If remote mode is active, serialize the prompt and publish a `TYPE_PERMISSION_REQUEST` JSON packet onto the WebRTC `control` data channel.
- Block the active tool execution goroutine on an internal Go `chan bool` channel.
- Implement a 5-minute timeout case using `select` and `time.After`. Fail-closed (return false) if the timeout triggers before a response is received.

### Remote Response Handlers
#### [MODIFY] [internal/remote/handlers.go](file:///Users/human/code/powerword/internal/remote/handlers.go)
- Listen for incoming WebRTC `control` channel inputs.
- Map `TYPE_PERMISSION_RESPONSE` packets to unblock the waiting guard channel with the boolean approval state.

---

## Verification Plan

### Automated Tests
- In `guard_test.go`, mock active WebRTC channels and assert that the validator blocks execution, resolves successfully on remote `true` inputs, and fails-closed on `false` or timeouts.

### Manual Verification
- Prompt Powerword to run a shell command.
- Confirm the local shell halts with "Awaiting remote authorization...".
- Tap "Approve" on the mobile device lock screen or dashboard and confirm the CLI immediately resumes execution.
