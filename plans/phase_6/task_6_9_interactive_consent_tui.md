# plan: Task 6.9: Rich Interactive Consent TUI

**Status:** Open (Issue #TBD)

This task replaces standard command-line prompts with a Bubbletea-based Terminal User Interface (TUI) showing detailed tool execution details, differences, and permission configurations before authorization is granted.

## User Review Required

> [!NOTE]
> The consent interface must support mouse click interactions, scrollable content panels, and keyboard navigation.

> [!IMPORTANT]
> **Remote Approval Integration**:
> To support the mobile companion app (Lamplighter Task 3.7), the consent manager must detect if a remote signaling tunnel is active (e.g., via the configured Firebase RTDB tunnel). If active, it will publish the approval request details to the Firebase path `/tunnels/{tunnelId}/approval` and block, waiting for a response, while displaying a "Waiting for Remote Approval..." status on the local TUI. A local user can still manually override or cancel the wait.

## Proposed Changes

### TUI Consent Manager

#### [NEW] [consent.go](file://../../internal/loop/consent.go)
- [ ] Build a Bubbletea model displaying tool details, risk level badges, and execution options (e.g. Always Allow, Allow Once, Deny).
- [ ] Support rendering file differences if the tool requests write permissions.
- [ ] Implement remote approval handler: if a signaling client is active, serialize the tool request to `/tunnels/{tunnelId}/approval` with fields: `action`, `target`, `summary`, `status` ("pending").
- [ ] Listen to updates on `/tunnels/{tunnelId}/approval/status` and resume the loop automatically if approved or denied.
- [ ] Render a "Waiting for remote approval..." overlay or status bar on the TUI, allowing local keyboard overrides to approve/deny locally.

#### [MODIFY] [terminal.go](file://../../internal/loop/terminal.go)
- [ ] Hook the consent views into the command-line interaction loop.
- [ ] Inject the signaling client interface (if active) into the consent coordinator to facilitate the remote RTDB approval handshake.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/loop/...` checking state changes and rendering components of the consent model.
- [ ] Add mock tests for signaling state transitions (e.g., waiting, remote approved, remote denied) to ensure loop execution matches remote responses.

### Manual Verification
- [ ] Run an execution that requests filesystem edits, and verify that the rich consent interface loads.
- [ ] Spin up a mock signaling daemon (e.g. `simulate_signaling.js` or manual console edits), verify that a tool request publishes a payload to the Firebase path, and mock-approve it to verify the powerword execution loop proceeds automatically.
