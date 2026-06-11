# plan: Task 3.6: KDP Book Geometry & PDF Validator Plugin (pw-mcp-kdp-math)

**Status:** Completed

This task implements a native Go-based MCP server (`pw-mcp-kdp-math`) that calculates exact Amazon KDP layouts (trim size, safety margin, bleed, spine width) and analyzes compiled PDFs to ensure technical compliance before publishing.

## User Review Required

- **Go Version:** Go 1.26.4
- **PDF Parser Library Choice:** `rsc.io/pdf` was selected as a pure Go dependency. We implemented `safeReaderAt` (a wrapper around `io.ReaderAt`) to virtually insert a `%` right before `%EOF` to bypass a hardcoded parser bug in the library's `read.go` without mutating physical files on disk. The virtual insertion buffer was expanded to 1024 bytes to handle extensive trailing whitespace before `%EOF`.
- **Sandbox Security Checks:** Incorporated path traversal protection (`checkSandbox` using `securejoin`) to reject any inputs that fall outside the workspace boundary.
- **Rounding Tolerances:** Set page dimensions matching tolerance threshold to `3.6pt` (0.05 inches) to prevent false negatives caused by floating-point rounding variations in PDF exporters.
- **Complexity Management:** Split parsing, cover validations, and interior validations into clean helper subroutines to ensure cognitive complexity of all functions remained below 20 and passed golangci-lint guidelines.


## Proposed Changes

### KDP Math Plugin Component
Created the new directory `internal/plugins/kdpmath/` containing the book geometry and validation logic.

#### [NEW] [kdpmath.go](../internal/plugins/kdpmath/kdpmath.go)
- Implemented layout calculations based on KDP hardcover and paperback specification equations.
- Integrated `rsc.io/pdf` page geometry reader.
- Exposed the following MCP tools:
  - `kdp_calculate_geometry`: Computes exact cover, interior page dimensions, safety margins, and spine thickness based on page count, binding type, paper type, and trim size.
  - `kdp_validate_pdf`: Parses an existing PDF to inspect its page count, dimensions in points, and ensures target bleed lines are met.
  - `kdp_generate_manifest`: Creates standard JSON layout configurations for BookBolt/Inkfluence.

#### [NEW] [kdpmath_test.go](../internal/plugins/kdpmath/kdpmath_test.go)
- Parameterized table-driven unit tests verifying math against official published sizing charts.
- Programmatic mock PDF generation to verify layout validations, page dimensions, inherited fields, and the `safeReaderAt` EOF virtual correction.

### Standalone MCP Server Binary
#### [NEW] [main.go](../cmd/pw-mcp-kdp-math/main.go)
- Standard stdio Model Context Protocol (MCP) server launching the three book geometry tools.
#### [NEW] [main_test.go](../cmd/pw-mcp-kdp-math/main_test.go)
- End-to-end integration tests using in-memory transports and clients.

### CLI Manifest Integration
#### [MODIFY] [config.go](../internal/config/config.go)
- Registered the `pw-mcp-kdp-math` server config structure.
#### [MODIFY] [powerword.example.toml](../powerword.example.toml)
- Added template config `servers.kdp_math` calling the local command.
#### [MODIFY] [Makefile](../Makefile)
- Registered `KDP_MATH_PLUGIN` inside standard `build` and `install` Makefile scripts.

---

## Verification Plan

### Automated Tests
- **Package Tests:** `go test -v ./internal/plugins/kdpmath/...` (Passed)
- **Server Tests:** `go test -v ./cmd/pw-mcp-kdp-math/...` (Passed)
- **Lint Compliance:** `make lint` (Passed with 0 issues)
- **Coverage gating:** `make check-coverage` (Passed with **91.2%** statement coverage, meeting the >91% threshold)
