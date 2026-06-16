# plan: Task 3.17: Grayscale PDF Preflight Checker (`pw-mcp-pdfcheck`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-16
**Unit Test Coverage:** 93.2% (internal/plugins/pdfcheck), project-wide overall 91.1%

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
- Run unit tests and verify code coverage is $\ge 91\%$:
  ```bash
  go test -race -v ./internal/plugins/pdfcheck/...
  make check-coverage
  make lint
  ```
- Run integration tests (launches the compiled binary as a subprocess and tests the MCP stdio handshake/calls):
  ```bash
  make test-integration
  ```
- Test cases validating `enforce_grayscale`:
  - Unit test `TestValidatePDFPreflight_EnforceGrayscale` and `TestValidatePDFPreflight_EnforceGrayscale_Operators` verify logic inside `validator.go`.
  - Integration test `TestMCP_PdfcheckPlugin_Grayscale` verifies the command line entrypoint and plugin lifecycle.

### Manual Verification

1. **Build the `pw-mcp-pdfcheck` plugin binary:**
   ```bash
   go build -o bin/pw-mcp-pdfcheck ./cmd/pw-mcp-pdfcheck
   ```

2. **Run direct validation via stdio JSON-RPC:**
   Execute the compiled binary:
   ```bash
   ./bin/pw-mcp-pdfcheck
   ```
   
   To bypass `"method \"tools/call\" is invalid during session initialization"`, you must perform the MCP initialization handshake first. Paste these JSON payloads in sequence on stdin (be sure to replace `"path/to/your/test.pdf"` in Step C with a real local PDF path):

   **A. Send `initialize` Request:**
   ```json
   {"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":0}
   ```
   *(The server will respond with capabilities.)*

   **B. Send `notifications/initialized` Notification:**
   ```json
   {"jsonrpc":"2.0","method":"notifications/initialized"}
   ```

   **C. Send the `tools/call` Request:**
   ```json
   {
     "jsonrpc": "2.0",
     "method": "tools/call",
     "params": {
       "name": "validate_pdf",
       "arguments": {
         "pdf_path": "path/to/your/test.pdf",
         "expected_width_inches": 6.0,
         "expected_height_inches": 9.0,
         "enforce_grayscale": true
       }
     },
     "id": 1
   }
   ```
   **Expected Output:**
   - For a grayscale manuscript: The JSON result stdout should report `"valid": true` with zero color errors.
   - For a color manuscript: The JSON result stdout should report `"valid": false` and contain diagnostic strings indicating exactly which pages contain RGB images or RGB/CMYK drawing operations.

3. **Verify via integrated `powerword` run:**
   Register the plugin in `powerword.toml`:
   ```toml
   [[mcp.servers]]
   name = "pdfcheck"
   command = "./bin/pw-mcp-pdfcheck"
   ```
   Invoke `powerword` and ask it to run validation on the target PDF using the `validate_pdf` tool, confirming the agent handles consent prompts and executes the tool end-to-end.

