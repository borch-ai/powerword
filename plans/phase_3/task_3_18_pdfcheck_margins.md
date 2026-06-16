# plan: Task 3.18: Gutter & Safe-Zone Margin Analyzer (`pw-mcp-pdfcheck`)

**Status:** Open (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Implement dynamic gutter and page margin safe-zone checks in `pw-mcp-pdfcheck` to verify that text and structural elements do not run too close to the binding gutter or outer page trim boundaries.

## User Review Required

> [!IMPORTANT]
> **Page Element Inspection**:
> Parsing the spatial bounding boxes of individual text segments and characters requires deep PDF stream parsing using `rsc.io/pdf` page text extraction. This may be slower on large manuscripts, so it should be optimized or only run on a sample/range of pages if requested.

## Proposed Changes

### MCP Server Tool Definition

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-pdfcheck/main.go)
- Add optional input parameters to `validate_pdf` schema:
  - `min_gutter_inches` (number, optional): Minimum binding gutter required on the inside margins.
  - `min_margin_inches` (number, optional): Minimum safe zone margin required from the outer trim lines.

### Safe-Zone Auditing Logic

#### [MODIFY] [validator.go](file://../../internal/plugins/pdfcheck/validator.go)
- In `ValidatePDFInput` struct, add fields:
  - `MinGutterInches *float64`
  - `MinMarginInches *float64`
- Implement helper function `validateTextSafeZones(page pdf.Page, pageNum int, isLeftPage bool, gutterPts, marginPts float64) []string`:
  - Extract all text coordinates and characters from the page stream.
  - Assert that no character coordinates fall within the horizontal/vertical bounding boxes of the unsafe margins:
    - Left page inside binding gutter: rightmost edge of the page.
    - Right page inside binding gutter: leftmost edge of the page.
    - Top, bottom, and outer edges of both pages.
- Integrate margin safe-zone auditing into the main validation loop.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/pdfcheck/...` and verify code coverage is $\ge 91\%$.
- Test cases validating margins:
  - Page with text within safe boundary returns valid.
  - Page with text crossing the gutter margin boundary returns invalid with line content details.

### Manual Verification
- Run `./bin/powerword` and call `validate_pdf` on a compiled PDF interior manuscript with safe-zone parameters enabled, verifying output verdict.
