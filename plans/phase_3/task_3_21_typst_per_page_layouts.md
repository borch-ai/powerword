# plan: Task 3.21: Typst Per-Page Layout Overrides

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** June 19, 2026
**Unit Test Coverage:** 91.2% (codebase-wide statement coverage)

Extend the `pw-mcp-typst` compiler to support per-page layout styles (e.g. `facing-pages`, `facing-pages-flipped`, `full-bleed`) inside compiled book interior PDFs, configured via comments inside the Markdown manuscript (e.g., `<!-- Layout: facing-pages -->`).

## User Review Required

> [!NOTE]
> This change is backwards-compatible. If a page does not specify a layout comment, it defaults to the global book layout.

---

## Proposed Changes

### Typst Plugin Parser

#### [MODIFY] [parser.go](file://../../internal/plugins/typst/parser.go)

- Add `Layout` string field to `Page` struct.
- Update `ParseManuscript` to detect comments containing layout instructions:
  - If a line matches `<!-- Layout: <layout> -->` (case-insensitive), extract the layout type and assign it to the current page's `Layout` field.
  - Do not skip layout comments in `shouldSkip` if they need to be processed, or process comments before checking `shouldSkip`.

### Typst Plugin Template & Renderer

#### [MODIFY] [template.go](file://../../internal/plugins/typst/template.go)

- Add `Layout` string field to `InteriorPage` struct.
- In `interiorTemplateSrc`, set a local `$layout` variable per page inside the page loop, defaulting to the global default `$.Layout` if `page.Layout` is empty:

  ```typst
  {{range $page := .Pages}}
  {{$layout := $page.Layout}}
  {{if eq $layout ""}}{{$layout = $.Layout}}{{end}}
  {{if eq $layout "facing-pages"}}
    ...
  ```

### Typst MCP Server Command Handler

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-typst/main.go)

- In `compileFromMarkdownFile`, map `p.Layout` from parsed `Page` to `InteriorPage.Layout` when mapping elements:

  ```go
   var interiorPages []typst.InteriorPage
   for _, p := range pages {
    ip := typst.InteriorPage{
     Text:      p.Text,
     Layout:    p.Layout,
    }
  ```

---

## Verification Plan

### Automated Tests

- Update [typst_test.go](file://../../internal/plugins/typst/typst_test.go) and [main_test.go](file://../../cmd/pw-mcp-typst/main_test.go) to verify:
  - `ParseManuscript` parses layout comments correctly.
  - Template generation outputs the expected Typst layout markup when pages specify different layout configurations.
  - Overall coverage remains above **91%**.

  ```bash
  go test -v ./internal/plugins/typst/...
  go test -v ./cmd/pw-mcp-typst/...
  ```

### Manual Verification

1. Call `pw-mcp-typst`'s `compile_interior` tool with a markdown manuscript containing layout comments on specific pages.
2. Verify that the output PDF page layouts match the layout comment definitions (e.g. split text/image page layouts).
