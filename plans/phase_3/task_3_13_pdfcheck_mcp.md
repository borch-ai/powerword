# plan: Task 3.13: Print-Ready PDF Preflight Inspector (`pw-mcp-pdfcheck`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-16
**Unit Test Coverage:** 91.1%

Implement `pw-mcp-pdfcheck`, a native Go MCP server that performs preflight validation checks on compiled PDF book manuscripts to ensure compliance with Amazon KDP paperback requirements. Primary consumer: the Pithos `assemble` pipeline.

## User Review Required

> [!IMPORTANT]
> **CLI Dependencies**:
> PDF page geometry and DPI analysis can be performed via Go libraries, but deep inspection of individual page color profiles (e.g., detecting if any image contains RGB color elements) and listing embedded fonts is most robust when wrapping native tools. This server will leverage standard CLI utilities (like `pdfinfo` and `pdfimages` from `poppler-utils`) if present, throwing clear warnings/errors to configure them if they are missing.

## Proposed Changes

### New Binary: `cmd/pw-mcp-pdfcheck/`

#### [NEW] [main.go](file://../../cmd/pw-mcp-pdfcheck/main.go)

Standard MCP server entry point registering the `validate_pdf` tool:

```go
func main() {
    srv := mcp.NewServer(&mcp.Implementation{
        Name:    "pw-mcp-pdfcheck",
        Version: "0.1.0",
    }, nil)
    srv.AddTool(&mcp.Tool{Name: "validate_pdf", ...}, handleValidatePDF)
    if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

### MCP Tools

#### Tool: `validate_pdf`

**Input schema:**

```json
{
    "pdf_path": "string (path to target PDF file)",
    "expected_width_inches": "number",
    "expected_height_inches": "number",
    "bleed_inches": "number (default: 0.125)",
    "min_dpi": "integer (default: 300)",
    "enforce_embedded_fonts": "boolean (default: true)",
    "enforce_cmyk": "boolean (default: false)"
}
```

**Implementation:**

1. Check that the PDF exists and is a readable file.
2. Read the page count, crop box, and media box specifications of the PDF file to verify exact paper dimensions match:
   `width = expected_width_inches + (bleed_inches * 2)`
   `height = expected_height_inches + (bleed_inches * 2)`
3. Extract and check properties of all embedded image objects to verify that their native dimensions match or exceed the target `min_dpi` (standard paperback requirement is 300 DPI).
4. Run font analysis on the document structure to assert that every single referenced font is marked as "Embedded" or "Embedded Subset".
5. Optionally verify that the color spaces of embedded images do not contain RGB coordinates if `enforce_cmyk` is enabled.
6. Return a structured JSON verdict:

   ```json
   {
       "valid": true,
       "page_count": 32,
       "dimensions": "8.625 x 8.625 in (with bleed)",
       "errors": [],
       "warnings": []
   }
   ```

### PDF Validation Logic

#### [NEW] [validator.go](file://../../internal/plugins/pdfcheck/validator.go)

Core validation functions analyzing page configurations, image sizes, and font mappings.

### Tests

#### [NEW] [validator_test.go](file://../../internal/plugins/pdfcheck/validator_test.go)

- Unit tests compiling dynamic test PDF containers containing embedded elements to verify parser accuracy.
- Mocking tool subprocess checks.
- 91%+ coverage.

---

## Verification Plan

### Automated Tests

- `go test -race ./cmd/pw-mcp-pdfcheck/... ./internal/plugins/pdfcheck/...`
- `make check-coverage` — ≥91%

### Manual Verification

1. `./bin/pw-mcp-pdfcheck` — starts and waits for stdio input.
2. Pass a valid children's book interior PDF compiled by Typst; verify that the validator returns a successful result showing correct page sizes.
3. Pass a modified PDF containing a non-embedded font or a low-resolution image (e.g., 72 DPI); verify that the tool returns a failed verdict with specific error descriptions.
