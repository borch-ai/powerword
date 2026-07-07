# plan: Task 6.30: Interactive TUI Chat Shell Mode (`powerword chat`)

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

## Goal Description

Build a rich interactive terminal user interface (TUI) command `powerword chat` using Bubbletea and Lipgloss. It replaces standard single-turn command executions with a persistent, styled chat session that supports syntax-highlighted markdown rendering, collapsible log blocks for background tool runs, and real-time token/cost budget indicator meters.

## User Review Required

> [!IMPORTANT]
> **TTY Enforcement**:
> TUI mode is strictly interactive. If the CLI is run in a non-TTY environment (e.g. CI pipeline or headless stdin), the TUI must fallback to standard headless JSON or standard streaming print.

---

## Proposed Changes

### CLI Component (`cmd/powerword`)

#### [NEW] [chat.go](file://../../cmd/powerword/chat.go)
* Define the `powerword chat` command.
* Initialize a Bubbletea application using a custom chat model.
* Wire input keys (Enter to submit, Up/Down to scroll, Esc to quit/pause).
* Render a Split screen viewport:
  * Left/Top pane: Chat history render using a Lipgloss-styled markdown renderer.
  * Right/Bottom pane: Running status panel detailing active MCP tool calls, duration, and a cost accounting meter.

### TUI Component (`internal/tui`)

#### [NEW] [tui.go](file://../../internal/tui/tui.go)
* Build the Bubbletea Update, View, and Init functions.
* Implement custom viewport scroll and text input bindings.
* Implement styled blocks to render collapsible JSON details for tool inputs and output values.

---

## Verification Plan

### Automated Tests
* Create unit tests in `internal/tui/tui_test.go` verifying:
  * Keypress events are mapped correctly to model messages.
  * Collapsible view states toggle correctly.
  * Graceful fallback when terminal is non-TTY.

### Manual Verification
1. Run `go build -o bin/powerword ./cmd/powerword`.
2. Execute `bin/powerword chat` and verify that the TUI initializes.
3. Submit a prompt and observe that MCP tool executions are rendered as collapsible elements while the cost tracker updates.
