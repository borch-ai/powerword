# plan: Task 3.15: Typst Mixed Layout Templates

**Status:** Completed (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** 2026-06-18
**Unit Test Coverage:** 94.0%

Support layout styles (e.g. left-hand text page, right-hand image page) inside `pw-mcp-typst`'s compilation tools.

## User Review Required

> [!NOTE]
> Adds a new optional parameter `layout` inside `compile_interior` schemas to support mixed page templates: `"full-bleed"`, `"facing-pages"`, and `"facing-pages-flipped"`.

---

## Proposed Changes

### Configuration
Extend the tool schemas to support layout options.

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-typst/main.go)
- Register `"layout"` optional field inside `compileInteriorSchema`.
- Extend `interiorArgs` struct to capture `"layout"` parameter.
- Default to `"full-bleed"` in `applyInteriorDefaults`.
- Validate `"layout"` value inside `parseAndSanitizeInterior`.

### Typst Template
Support rendering split templates and conditional blocks.

#### [MODIFY] [template.go](file://../../internal/plugins/typst/template.go)
- Add `Layout` field to `InteriorParams` struct.
- Refactor `interiorTemplateSrc` to generate conditional layout pages using Typst code depending on the `layout` setting.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/typst/...` to verify the page generation outputs correct Typst files for each layout option.
- Run `go test -v ./cmd/pw-mcp-typst/...` to verify schema validation and mock compile runs.
- Assert package coverage is at least **91%**.

### Manual Verification
- Compile and call `compile_interior` with split page layouts, and verify output PDF formatting.
