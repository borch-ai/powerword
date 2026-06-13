# plan: Task 3.12: EPUB Publication Builder (`pw-mcp-epub`)

**Status:** Open (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Implement `pw-mcp-epub`, a native Go MCP server that compiles parodic manuscripts and illustration assets into spec-compliant EPUB digital publications. Primary consumer: the Pithos `digital-export` pipeline.

## User Review Required

> [!NOTE]
> **EPUB Validation Rules**:
> The generated EPUB will conform strictly to the EPUB 3 specification. We will include basic metadata validation (such as ensuring unique UUID identifiers, creator credits, and correct MIME type headers) to prevent Kindle validation failures.

## Proposed Changes

### New Binary: `cmd/pw-mcp-epub/`

#### [NEW] [main.go](file://../../cmd/pw-mcp-epub/main.go)
Standard MCP server entry point registering the `compile_epub` tool:
```go
func main() {
    srv := mcp.NewServer(&mcp.Implementation{
        Name:    "pw-mcp-epub",
        Version: "0.1.0",
    }, nil)
    srv.AddTool(&mcp.Tool{Name: "compile_epub", ...}, handleCompileEPUB)
    if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

### MCP Tools

#### Tool: `compile_epub`
**Input schema:**
```json
{
    "manuscript_path": "string (path to manuscript Markdown/HTML source)",
    "images_dir": "string (directory containing book illustration PNGs)",
    "output_path": "string (desired destination for compiled EPUB)",
    "title": "string",
    "author": "string",
    "language": "string (default: 'en')",
    "stylesheet_path": "string (optional custom CSS template)"
}
```

**Implementation:**
1. Read the manuscript source file and parse chapters/stanzas into separate XHTML content documents.
2. Embed the images found in `images_dir` into the EPUB package and write appropriate XHTML image references.
3. Dynamically generate the standard package metadata files:
   - `mimetype` (containing strictly `application/epub+zip` with no compression or trailing newline)
   - `META-INF/container.xml` (pointing to the OPF content package file)
   - `EPUB/content.opf` (defining the package manifest, spine order, metadata, and guide items)
   - `EPUB/toc.xhtml` (defining the HTML5 table of contents navigation document)
4. Zip the entire generated structure, applying no-compression specifically to the first `mimetype` file in the archive to satisfy the EPUB container specification.
5. Verify the compiled `.epub` file exists and is non-empty.

### EPUB Core Packages

#### [NEW] [epub.go](file://../../internal/plugins/epub/epub.go)
Go library implementing EPUB container packaging and file structure layouts.

### Tests

#### [NEW] [epub_test.go](file://../../internal/plugins/epub/epub_test.go)
- Unit tests verifying correct OPF manifest xml generation.
- Tests validating that standard zip outputs conform to mimetype uncompressed first-entry restrictions.
- 91%+ test coverage.

---

## Verification Plan

### Automated Tests
- `go test -race ./cmd/pw-mcp-epub/... ./internal/plugins/epub/...`
- `make check-coverage` — ≥91%

### Manual Verification
1. `./bin/pw-mcp-epub` — starts and waits for stdio input.
2. Send a `compile_epub` tool call with a test manuscript and verify a valid `.epub` is written.
3. Validate output file with `epubcheck` command-line utility to confirm 100% compliance with EPUB 3 requirements.
