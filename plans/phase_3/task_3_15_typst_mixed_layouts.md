# plan: Task 3.15: Typst Mixed Layout Templates

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Support layout styles (e.g. left-hand text page, right-hand image page) inside `pw-mcp-typst`'s compilation tools.

## User Review Required

> [!NOTE]
> Adds a new optional parameter `layout` inside `compile_interior` schemas to support mixed page templates.

## Proposed Changes

### Configuration
Extend the tool schemas to support layout options.

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-typst/main.go)
- Support a `layout` field inside `compile_interior` schemas.

### Typst Template
Support rendering split templates and conditional blocks.

#### [MODIFY] [template.go](file://../../internal/plugins/typst/template.go)
- Update `interiorTemplateSrc` to accept and render split layout types.

---

## Verification Plan

### Automated Tests
- Add unit tests verifying multiple template rendering modes.

### Manual Verification
- Compile and call `compile_interior` with split page layouts, and verify output PDF formatting.
