# plan: Task 3.14: Typst Verse Line Break Formatting

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-15
**Unit Test Coverage:** 91.20%

Preserve single newline markers within Markdown manuscript stanzas and format them as Typst breaks (` \\\n `) during compilation.

## User Review Required

> [!NOTE]
> Formatting is executed inside the parser implementation before compiling the markup.

## Proposed Changes

### Parser

Modify `ParseManuscript` parser to preserve line breaks or format them as Typst-friendly layout breaks.

#### [MODIFY] [parser.go](file://../../internal/plugins/typst/parser.go)

- Enhance `cleanPages` to replace `\n` in `Text` fields with Typst line break syntax (` \\\n `).

---

## Verification Plan

### Automated Tests

- Add unit test verifying that parsed manuscript stanzas maintain explicit line breaks.

### Manual Verification

- Compile `pw-mcp-typst` and run `compile_interior` to verify that output PDF stanzas display as separate lines.
