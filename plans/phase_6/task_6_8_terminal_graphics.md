# plan: Task 6.8: Inline Graphics Rendering

**Status:** Open (Issue #TBD)

This task implements native terminal graphic protocols (iTerm2, Kitty graphics, and Sixel) inside the output engine, enabling media-related MCP plugins to display images directly inside the terminal.

## User Review Required

> [!NOTE]
> Graphic rendering requires fallback support for terminal clients that do not support iTerm2/Kitty protocol (printing standard file URLs instead of binary image codes).

## Proposed Changes

### Output Engine

#### [MODIFY] [terminal.go](file://../../internal/loop/terminal.go)

- [ ] Add terminal feature detection (querying environment variables and terminal response codes for image protocol support).
- [ ] Implement encoder sequences to format PNG/JPEG bytes into iTerm2, Kitty, and Sixel escape strings.
- [ ] Intercept image markdown links in text streams and render them inline.

---

## Verification Plan

### Automated Tests

- [ ] Run `go test ./internal/loop/...` verifying that escape sequences are correctly constructed and match protocol specs.

### Manual Verification

- [ ] Run Powerword in a compatible terminal (e.g. iTerm2/WezTerm) and ask the imagegen tool to output an image, verifying the visual render.
