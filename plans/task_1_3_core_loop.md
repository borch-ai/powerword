# Task 1.3: Core Loop & Streaming Output Engine - Completed

Implement the primary non-interactive CLI execution pipeline. Capture prompt streams from the LLM client and render them cleanly in real-time, utilizing styled markdown formatting inside the terminal.

## Design and Implementation Decisions

- **Go Version Used**: Go `1.26.4`
- **Streaming Parser & Formatter**: Rather than rendering the entire text after completion (e.g. using Glamour), a custom `TerminalFormatter` was built to parse and print tokens character-by-character on the fly:
  - Highlights code blocks with borders (`╭──`, `│`, `╰──`) in blue/cyan.
  - Formats headings level 1 to 6 using colored unicode blocks (`█`, `▓`, `▒`, `░`).
  - Supports bold (`**`) and italic (`*`) text toggle states.
  - Recognizes `<think>` and `</think>` tags to format thinking blocks in dim gray and italics with a brain emoji indicator.
- **Dynamic Terminal Wrapping & Indentation**:
  - Implements word-wrapping by buffering words statefully.
  - Buffers spaces statefully to eliminate trailing spaces on wrapped lines.
  - Dynamically aligns wrapped lines to bullet list items (` • `), ordered list items (` 1. `), thinking blocks, and headers by retaining prefix indent levels.
  - Integrates Unix ioctl for terminal width detection with a cross-platform Windows fallback.
- **Dependency Injection**:
  - Utilizes a package-level function pointer `config.Runner` to register the loop execution from the `main` package. This avoids package import cycles between `internal/config` (CLI command definition) and `internal/loop` (core execution).

## Proposed Changes

### Core Loop & Output Formatter

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Implements `RunLoop(ctx context.Context, cfg *config.Config, prompt string) error`.
- Sets up client streaming, instantiates `TerminalFormatter`, and writes text chunks.

#### [NEW] [terminal.go](file:///Users/human/code/powerword/internal/loop/terminal.go)
- Stateful stream formatting state machine tracking markdown token boundaries and layout/style transitions.

#### [NEW] [terminal_width.go](file:///Users/human/code/powerword/internal/loop/terminal_width.go)
- Unix ioctl window size check.

#### [NEW] [terminal_width_windows.go](file:///Users/human/code/powerword/internal/loop/terminal_width_windows.go)
- Windows terminal size fallback.

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Calls `Runner(cmd.Context(), Active, prompt)` in the root command's `RunE` function.

#### [MODIFY] [main.go](file:///Users/human/code/powerword/cmd/powerword/main.go)
- Registers `loop.RunLoop` to `config.Runner`.

---

## Verification Plan

### Automated Tests
- Run `make check-coverage` and `make lint` to verify that all targets are clean.
- Unit tests in `internal/loop/terminal_test.go` and `internal/loop/loop_test.go`:
  - Verified word wrapping at line length boundaries (character and word wrapping).
  - Verified indentation preservation on lists and headers.
  - Verified multi-byte UTF-8 partial rune streaming buffers.
  - Verified mock LLM client streams and error chunk propagation.
  - Verified 92.00% unit test coverage across `./internal/...`.
