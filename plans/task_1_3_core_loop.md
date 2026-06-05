# Task 1.3: Core Loop & Streaming Output Engine

Implement the primary non-interactive CLI execution pipeline. Capture prompt streams from the LLM client and render them cleanly in real-time, utilizing styled markdown formatting inside the terminal.

## User Review Required

> [!TIP]
> Rendering Markdown: For rich terminal rendering, we can utilize a library like `github.com/charmbracelet/glamour` for raw markdown rendering, or design our own clean, streaming-compatible token printer.
> Let's start with a streaming token printer that highlights code block structures for immediate responsiveness.

## Proposed Changes

### Core Loop & Output Formatter

#### [NEW] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Implements the primary `RunLoop(ctx context.Context, config *config.Config, prompt string) error` function.
- Orchestrates loading configuration, instantiating the LLM client, passing message arrays, and directing output to the CLI stream handler.

#### [NEW] [terminal.go](file:///Users/human/code/powerword/internal/loop/terminal.go)
- Handles real-time terminal print operations.
- Intercepts streaming tokens, detects code block tokens to apply terminal style overrides (e.g. bold, colored headers, italics for thinking blocks), and flushes stdout reliably.

---

## Verification Plan

### Automated Tests
- Mock prompt responses that stream text in chunks (including markdown blocks and emojis) to test layout formatting.
- Unit tests verifying terminal widths are handled correctly for line-wrapping.

### Manual Verification
- Execute a query using Gemini/OpenAI (e.g. `powerword "write a python function to fetch status codes"`) and verify that formatting matches standard markdown styling in standard terminal setups.
