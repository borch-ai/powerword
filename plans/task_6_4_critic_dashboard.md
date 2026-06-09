# Task 6.4: Local Critic Mobile Dashboard Integration

**Status:** Open (Issue #31)

Integrate the `powerword review` output (Local Critic reports) with the active remote connection, streaming Markdown feedback tables and Git diff summaries to the mobile device for review before a code push is authorized.

## User Review Required

> [!TIP]
> This requires the mobile PWA to support receiving complex Markdown strings and rendering them appropriately.

## Proposed Changes

### Review Pipeline
#### [MODIFY] [internal/review/critic.go](file:///Users/human/code/powerword/internal/review/critic.go)
- After the Critic completes its checks and compiles the markdown analysis report, check if a remote WebRTC session is active.
- Extract the current branch Git diff.
- Bundle the report and diff, and dispatch it as a `TYPE_CRITIC_REPORT` payload over the WebRTC `filesystem` data channel.

### Interactive Confirmations
#### [MODIFY] [cmd/powerword/review.go](file:///Users/human/code/powerword/cmd/powerword/review.go)
- Await remote confirmation from the dashboard before finalizing the push hook.
- Bind mobile approval actions directly to execute the underlying `git push` sequence.

---

## Verification Plan

### Manual Verification
- Run a Git pre-push checklist trigger.
- Confirm that the mobile PWA receives a notification: "Critic review completed".
- Review the criticism tables and git diff in the dashboard UI.
- Tap "Approve & Push" to trigger the git push.
