# plan: Task 6.9: Rich Interactive Consent TUI

**Status:** Open (Issue #TBD)

This task replaces standard command-line prompts with a Bubbletea-based Terminal User Interface (TUI) showing detailed tool execution details, differences, and permission configurations before authorization is granted.

## User Review Required

> [!NOTE]
> The consent interface must support mouse click interactions, scrollable content panels, and keyboard navigation.

## Proposed Changes

### TUI Consent Manager

#### [NEW] [consent.go](file://../../internal/loop/consent.go)
- [ ] Build a Bubbletea model displaying tool details, risk level badges, and execution options (e.g. Always Allow, Allow Once, Deny).
- [ ] Support rendering file differences if the tool requests write permissions.

#### [MODIFY] [terminal.go](file://../../internal/loop/terminal.go)
- [ ] Hook the consent views into the command-line interaction loop.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/loop/...` checking state changes and rendering components of the consent model.

### Manual Verification
- [ ] Run an execution that requests filesystem edits, and verify that the rich consent interface loads.
