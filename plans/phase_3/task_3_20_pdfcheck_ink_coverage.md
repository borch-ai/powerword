# plan: Task 3.20: Total Area Coverage Ink Density Analyzer (`pw-mcp-pdfcheck`)

**Status:** Open (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Implement Total Area Coverage (TAC) ink limit auditing in `pw-mcp-pdfcheck` to check that print-ready PDF files do not exceed maximum ink density limits (e.g. 240% for KDP paperbacks), preventing smudging and press tearing.

## User Review Required

> [!IMPORTANT]
> **Ghostscript Dependency**:
> Auditing exact pixel separations requires rendering the PDF raster layers and calculating the sum of color channels ($C + M + Y + K$). This check will run a local `gs` (Ghostscript) command in the background if installed, returning a warning if it is missing on the system.

## Proposed Changes

### MCP Server Tool Definition

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-pdfcheck/main.go)
- Add optional input parameter `max_ink_coverage` (integer, optional, default: 240) to `validate_pdf` tool.

### Ink Coverage Analysis Logic

#### [MODIFY] [validator.go](file://../../internal/plugins/pdfcheck/validator.go)
- In `ValidatePDFInput` struct, add field `MaxInkCoverage *int` mapping to `max_ink_coverage`.
- Implement function `checkInkDensity(ctx context.Context, pdfPath string, limit int, res *ValidatePDFResult)`:
  - Verify Ghostscript is present: `exec.LookPath("gs")`.
  - Invoke `gs` to dump the PDF separations or render pages to a low-DPI CMYK TIFF/PNG file using `-sDEVICE=tiffsep` or `-sDEVICE=png16m`.
  - Parse the output pixel values to compute ink coverage: `coverage = Cyan% + Magenta% + Yellow% + Black%`.
  - Log a validation error if any contiguous region exceeds the limit.
  - Clean up all generated separations/renderings.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/pdfcheck/...` and verify code coverage is $\ge 91\%$.
- Test cases validating ink limits:
  - Simulated high density values parse correctly and return errors.
  - Fallback logic checks when `gs` is missing.

### Manual Verification
- Run `./bin/powerword` and execute `validate_pdf` with `max_ink_coverage = 240` on a PDF containing dark, saturated color images to verify detection.
