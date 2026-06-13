# plan: Task 3.11: Typst PDF Layout Plugin (`pw-mcp-typst`)

**Status:** Completed (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** 2026-06-13
**Unit Test Coverage:** 91.2%

Implement `pw-mcp-typst`, a native Go MCP server that invokes a local Typst binary to compile book manuscripts and illustration assets into print-ready PDFs conforming to KDP bleed, margin, and resolution specifications. Primary consumer: the Pithos `assemble` engine (Pithos Task 4.2).

> [!IMPORTANT]
> **This task unblocks Pithos Phase 4.** Pithos's `assemble` command is currently stalled because it lacks a PDF layout engine. This plugin is the dependency. Treat it as high priority relative to other Phase 3 Powerword tasks.

## User Review Required

> [!IMPORTANT]
> **Typst must be installed on the host system.** Document the installation requirement: `brew install typst` on macOS, `apt install typst` on Linux. Provide a clear error if `typst` binary is not on PATH.

> [!NOTE]
> **Typst vs LaTeX.** Typst is chosen over LaTeX because:
> - Compile time: Typst compiles a 32-page illustrated book in <2s; LaTeX takes 30–90s.
> - Dependency footprint: Typst is a single binary; LaTeX requires a TexLive installation (~4GB).
> - Error messages: Typst errors are human-readable; LaTeX errors are arcane.
> - KDP spec compliance: Typst's paper size and margin controls are explicit and predictable.

> [!NOTE]
> **KDP Interior Specs (Paperback, 8.5×8.5", as of 2026):**
> - Page size: 8.5×8.5 inches
> - Inside margins: 0.375" (pages ≤ 150) or 0.5" (pages 151–300)
> - Outside margins: 0.25" minimum
> - Bleed: 0.125" on all sides if full-bleed images
> - Min resolution: 300 DPI for embedded images

## Proposed Changes

### New Binary: `cmd/pw-mcp-typst/`

#### [NEW] [main.go](file://../../cmd/pw-mcp-typst/main.go)
Standard MCP server entry point registering two tools: `compile_interior` and `compile_cover`.

### MCP Tools

#### Tool: `compile_interior`
Compiles a multi-page illustrated book interior.

**Input schema:**
```json
{
    "manuscript_path": "string (path to .typ or .md source file)",
    "images_dir": "string (directory containing page illustration PNGs)",
    "output_path": "string (desired output PDF path)",
    "page_size": "string (default: '8.5in,8.5in')",
    "margin_inside": "string (default: '0.375in')",
    "margin_outside": "string (default: '0.25in')",
    "bleed": "string (default: '0.125in')",
    "font_family": "string (default: 'Georgia')"
}
```

**Implementation:**
1. Generate a Typst template file (`.typ`) from the input parameters, embedding image paths from `images_dir`.
2. Run `typst compile <template.typ> <output_path>` as a subprocess with `exec.CommandContext`.
3. Validate the output PDF exists and is non-empty.
4. Return `{"output_pdf": "<output_path>", "page_count": N}`.

#### Tool: `compile_cover`
Compiles a single-page book cover with spine.

**Input schema:**
```json
{
    "front_image_path": "string",
    "title": "string",
    "subtitle": "string",
    "author": "string",
    "page_count": "integer (for spine width calculation)",
    "output_path": "string"
}
```

**Spine width formula:** `spine_width_inches = page_count * 0.0025`

### Typst Template Generation

#### [NEW] [template.go](file://../../internal/plugins/typst/template.go)
Go `text/template` templates embedded via `go:embed` for both interior and cover layouts.

#### [NEW] [compiler.go](file://../../internal/plugins/typst/compiler.go)
```go
type Compiler struct {
    typstBin string
}

func (c *Compiler) Compile(ctx context.Context, templatePath, outputPath string) error { ... }
```

### Tests

#### [NEW] [typst_test.go](file://../../internal/plugins/typst/typst_test.go)
- Template generation produces valid `.typ` syntax.
- Compiler integration test (`//go:build integration`) requires real `typst` binary.
- Unit tests mock subprocess execution.
- 91%+ coverage on non-integration code.

---

## Verification Plan

### Automated Tests
- `go test ./internal/plugins/typst/...` (unit, subprocess mocked)
- `go test ./cmd/pw-mcp-typst/...` (unit, subprocess mocked)
- `RUN_INTEGRATION_TESTS=true go test ./internal/plugins/typst/...` (unit + integration tests, requires typst binary)
- `RUN_INTEGRATION_TESTS=true go test ./cmd/pw-mcp-typst/...` (unit + MCP integration tests calling actual typst compiler)

### Manual Verification
1. `./bin/pw-mcp-typst` — starts and awaits input.
2. Send `compile_interior` tool call with a 32-page manuscript; verify output PDF.
3. Open PDF in Preview; confirm margins, bleed marks, and image placement.
4. Run `pdfinfo` on output; verify resolution ≥ 300 DPI for embedded images.
5. Integrate with Pithos `assemble`: run end-to-end and get a KDP-ready interior PDF.
