# plan: Task 4.7: MCP ImageGen Cref Extensions

**Status:** Completed (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** 2026-06-17
**Unit Test Coverage:** 91.0%

This task extends the native image generation MCP server (`pw-mcp-imagegen`) to support character reference (`--cref`) and character weight (`--cw`) parameters.

Rather than compiling heavy storage SDKs directly into `pw-mcp-imagegen`, the `imagegen_generate` tool accepts a remote public URL for `cref_url`. If a local path is provided by the user, the Powerword ReAct agent loop (or sibling project client runner) is responsible for first calling `cloud_upload_file` on `pw-mcp-cloud` to upload the file, and then passing the returned public URL to `imagegen_generate`.

## User Review Required

> [!NOTE]
> **Orchestrator Responsibility:** The `imagegen_generate` tool itself does not perform file uploads. It expects a valid public HTTP/HTTPS URL for the `cref_url` input. The caller/agent is responsible for converting any local files to remote URLs using the `pw-mcp-cloud` tool beforehand.

---

## Proposed Changes

### ImageGen Component

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

- [x] Update `GenerateImage` method signature to accept optional `crefURL` (string) and `characterWeight` (integer pointer) arguments.
- [x] In `runMidjourney`, check if `crefURL` is not empty. If so, append `--cref <crefURL>` to the prompt.
- [x] In `runMidjourney`, check if `characterWeight` is not nil. If so, append `--cw <characterWeight>` to the prompt.

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-imagegen/main.go)

- [x] Extend the input schema of the `imagegen_generate` MCP tool:
  - Add `cref_url` (optional string, representing a public image URL).
  - Add `character_weight` (optional integer, min 0, max 100).
- [x] In `handleGenerate`, parse the new arguments from request parameters and pass them to `ImageGenService.GenerateImage`.

---

## Verification Plan

### Automated Tests

- [x] Add tests in `imagegen_test.go` to verify that `cref_url` and `character_weight` are parsed correctly, and that Midjourney prompt formatting includes the correct flags.
- [x] Assert the package test coverage meets or exceeds 91%.

### Manual Verification

- [x] Run `pw-mcp-imagegen` locally and call `imagegen_generate` with a remote image URL as the `cref_url` parameter. Verify that the final prompt passed to Midjourney includes the generated `--cref` URL and `--cw` parameter.
