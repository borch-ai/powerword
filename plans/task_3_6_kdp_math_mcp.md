# plan: Task 3.6: KDP Book Geometry & PDF Validator Plugin (pw-mcp-kdp-math)

**Status:** Open (Issue #TBD)

This task implements a native Go-based MCP server (`pw-mcp-kdp-math`) that calculates exact Amazon KDP layouts (trim size, safety margin, bleed, spine width) and analyzes compiled PDFs to ensure technical compliance before publishing.

## User Review Required

> [!IMPORTANT]
> PDF parsing in Go requires robust library selection. We propose utilizing `pdfcpu` APIs or `rsc.io/pdf` to extract page dimensions, crop boxes, and metadata without needing heavy, non-Go external binary dependencies.

## Proposed Changes

### KDP Math Plugin Component
Create a new directory `internal/plugins/kdpmath/` to contain the KDP validation engine.

#### [NEW] [kdpmath.go](file:///Users/human/code/powerword/internal/plugins/kdpmath/kdpmath.go)
- [ ] Implement layout calculations based on Amazon KDP hardcover and paperback specification equations.
- [ ] Integrate a PDF page geometry reader.
- [ ] Expose the following MCP tools:
  - `kdp_calculate_geometry`: Computes exact cover, interior page dimensions, safety margins, and spine thickness based on page count, binding type, and paper type.
  - `kdp_validate_pdf`: Parses an existing PDF to inspect its page count, dimensions in points, and ensures target bleed lines are met.
  - `kdp_generate_manifest`: Creates standard JSON layout configurations for BookBolt/Inkfluence.

#### [NEW] [kdpmath_test.go](file:///Users/human/code/powerword/internal/plugins/kdpmath/kdpmath_test.go)
- [ ] Unit tests to verify exact mathematical calculations against Amazon's official published sizing charts.
- [ ] Unit tests with mock or small generated PDFs to verify geometry validation logic.

### CLI Manifest Integration
#### [MODIFY] [internal/config/config.go](file:///Users/human/code/powerword/internal/config/config.go)
- [ ] Register the `pw-mcp-kdp-math` server within the native plugin registry under the config key `[plugins.kdp_math]`.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/plugins/kdpmath/...` to assert mathematical accuracy and page boundary checks.
- [ ] Enforce the 91% unit test coverage requirement.

### Manual Verification
- [ ] Compile a test children's book PDF.
- [ ] Run `powerword "validate that ./books/test-book.pdf is formatted correctly for a 24-page hardcover book with bleed"` and verify the tool checks the geometry and outputs validation details.
