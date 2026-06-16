# plan: Task 3.17: Grayscale PDF Preflight Checker (`pw-mcp-pdfcheck`)

**Status:** Open (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Extend the `pw-mcp-pdfcheck` toolset with grayscale enforcement to verify that manuscripts intended for black-and-white print editions only contain grayscale elements (no RGB or CMYK coordinates/color profiles).

## User Review Required

> [!NOTE]
> This check relies on the output of the standard `pdfimages -list` utility (which reports the color space of each image) and native Go PDF parsing via `rsc.io/pdf` to audit text/vector page color definitions.

## Proposed Changes

### MCP Server Tool Definition

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-pdfcheck/main.go)
- Add optional input parameter `enforce_grayscale` (boolean, default: `false`) to the `validate_pdf` tool schema:
  ```json
  "enforce_grayscale": {
      "type": "boolean",
      "description": "Optional enforcement that all elements and images use grayscale color spaces."
  }
  ```

### Validation Logic

#### [MODIFY] [validator.go](file://../../internal/plugins/pdfcheck/validator.go)
- In `ValidatePDFInput` struct, add field `EnforceGrayscale *bool` mapping to `enforce_grayscale`.
- In `checkImages` helper, pass `enforceGrayscale` to `parsePDFImagesOutput`.
- In `parsePDFImagesOutput`, if `enforceGrayscale` is enabled, check that the color column of each image is `gray` (or `mono`). If it is `rgb` or `cmyk`, append an error detailing the page and image color space.
- Audit vector paint operations to verify that no non-gray stroke or fill spaces are set.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/pdfcheck/...` and verify code coverage is $\ge 91\%$.
- Test cases validating `enforce_grayscale`:
  - Image list output containing `gray` returns valid.
  - Image list output containing `rgb` or `cmyk` returns invalid with proper error diagnostics.

### Manual Verification
- Execute `./bin/powerword` and invoke the `validate_pdf` tool with `enforce_grayscale = true` on a grayscale manuscript and a color manuscript, confirming correct validation verdicts.
