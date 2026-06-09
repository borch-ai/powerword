# Task 1.3: Core Loop & Streaming Output Engine

**Status:** Completed (Issue #38)

Implement the primary non-interactive CLI execution pipeline. Capture prompt streams from the LLM client and render them cleanly in real-time, utilizing styled markdown formatting inside the terminal.

## User Review Required

> [!NOTE]
> **Completed Implementation Details (Go Version: Go 1.23+)**:
> Rather than relying on static markdown rendering libraries (like Glamour), which require buffering the entire output before rendering, we implement a stateful, streaming-compatible token printer.
> This printer:
> - Highlights code blocks with beautiful Unicode box-drawing borders and color codes.
> - Preserves list-item indentation across wrapped lines.
> - Formats headers with decorative blocks (e.g. `█`, `▓`).
> - Toggles bold (`**`) and italic (`*`) text in real-time.
> - Identifies and styles `<think>` ... `</think>` blocks (common in reasoning models) with dimmed, italicized text and a brainstorming icon.
> - Word wrapping is handled dynamically. The formatter statefully buffers space tokens and wraps on word boundaries.
> - Supports multi-byte Unicode display-width calculations for correct wrapping of double-width CJK characters and emojis.
> - Integrates control character sanitization to prevent terminal control-sequence injection (escaping ESC `\x1b` to `^[` and converting other control characters/DEL `\x7f` to `\uFFFD`).
> - Implements robust, idempotent flushing and captures deferred write errors safely.

## Proposed Changes

### Core Loop & Output Formatter

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Updated `RunLoop` to set up client streaming, instantiate `TerminalFormatter`, and write text chunks.

#### [NEW] [terminal.go](file:///Users/human/code/powerword/internal/loop/terminal.go)
- Stateful stream formatting state machine tracking markdown token boundaries and layout/style transitions.

#### [NEW] [terminal_width.go](file:///Users/human/code/powerword/internal/loop/terminal_width.go)
- Unix ioctl window size check.

#### [NEW] [terminal_width_windows.go](file:///Users/human/code/powerword/internal/loop/terminal_width_windows.go)
- Windows terminal size fallback.

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Decouples loop execution and Cobra CLI parsing via `config.Runner` to avoid import cycles.

#### [MODIFY] [main.go](file:///Users/human/code/powerword/cmd/powerword/main.go)
- Registers `loop.RunLoop` to `config.Runner` on startup.

---

## Verification Plan

### Automated Tests
- Run `make check-coverage` and `make lint` to verify that all targets are clean.
- Unit tests in `internal/loop/terminal_test.go` and `internal/loop/loop_test.go`:
  - Verified word wrapping at line length boundaries (character and word wrapping).
  - Verified indentation preservation on lists and headers.
  - Verified multi-byte UTF-8 partial rune streaming buffers.
  - Verified mock LLM client streams and error chunk propagation.
  - Verified 91.60% unit test coverage across `./internal/...`.

### Manual Verification
- Execute a query using Gemini/OpenAI (e.g. `powerword "write a python function to fetch status codes"`) and verify that formatting matches standard markdown styling in standard terminal setups.
