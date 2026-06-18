# plan: Task 3.19: Spine Cover Wrap & Barcode Layout Checker (`pw-mcp-pdfcheck`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-18
**Unit Test Coverage:** 91.2% (overall statement coverage)

Implement cover wrap geometry checking and back cover barcode detection in `pw-mcp-pdfcheck` to verify that compiled book covers have the exact KDP-compliant dimensions (front cover, spine width, back cover, and bleeds) and contain a readable barcode.

## User Review Required

> [!NOTE]
> spine calculation requires a page count and paper type parameter. Barcode auditing will run barcode parsing patterns or inspect image metadata (or shell out to a local utility like `zbarimg` if installed) to verify the barcode has valid resolution and encodes the expected ISBN.

## Proposed Changes

### MCP Server Tool Definition

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-pdfcheck/main.go)
- Register tool `validate_cover_pdf` with input schema:
  - `pdf_path` (string, required)
  - `expected_width_inches` (number, required): Trim width of a single page (e.g. 6.0).
  - `expected_height_inches` (number, required): Trim height of a single page (e.g. 9.0).
  - `page_count` (integer, required): Total number of interior pages.
  - `paper_type` (string, required): "white" ($0.002252\text{ in/page}$), "cream" ($0.0025\text{ in/page}$), or "color" ($0.002347\text{ in/page}$).
  - `bleed_inches` (number, optional, default: 0.125).

### Cover Auditing Logic

#### [MODIFY] [validator.go](file://../../internal/plugins/pdfcheck/validator.go)
- Implement `ValidateCoverPDF(ctx context.Context, input ValidateCoverInput) (*ValidatePDFResult, error)`:
  - Calculate spine width: `spine = page_count * thickness_per_page`.
  - Calculate total expected cover width: `total_width = expected_width_inches * 2 + spine + bleed_inches * 2`.
  - Calculate total expected cover height: `total_height = expected_height_inches + bleed_inches * 2`.
  - Open PDF, check page count (must be exactly 1 for a flat KDP cover PDF).
  - Check that the page geometry matches the expected `total_width` and `total_height`.
  - Extract images located on the left half of the page (back cover wrap) and run a basic layout/pattern audit to confirm that a barcode is present and readable.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/pdfcheck/...` and verify code coverage is $\ge 91\%$.
- Test cases validating covers:
  - Valid cover geometries for a 200-page white paper book.
  - Geometry mismatch warnings when spine thickness or margins are incorrect.

### Manual Verification
- Compile a cover PDF using Typst for a 150-page manuscript, run `validate_cover_pdf`, and verify dimensions match calculations.
