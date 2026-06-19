# plan: Task 4.13: ImageGen Capabilities Handshake and Validation

**Status:** Completed
**Go Version:** 1.26.4
**Unit Test Coverage:** 91.1% (Actual)

This task introduces capability queries and input validation checks to the `pw-mcp-imagegen` server. When sibling projects query the generator, they should be able to determine feature compatibility (such as character or style reference support) dynamically and prevent failures during active production runs.

## User Review Required

> [!IMPORTANT]
> **Validation Policy:** Calling `imagegen_generate` with unsupported arguments (e.g. providing `cref_url` when the active backend is `openai` or style reference/sref when the active backend is `google`) will return an explicit tool execution error rather than silently ignoring the input parameters.

---

## Proposed Changes

### ImageGen Component

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)
- Define a new `Capabilities` struct in the `imagegen` package:
  ```go
  type Capabilities struct {
      Backend      string `json:"backend"`
      SupportsCref bool   `json:"supports_cref"`
      SupportsSref bool   `json:"supports_sref"`
  }
  ```
- Implement `GetCapabilities() Capabilities` on the `ImageGenService` struct:
  * Map capabilities: `supports_cref` is `true` for `midjourney`, `google`/`imagen`, and `veo` backends; `supports_sref` is `true` for `midjourney` (and `false` for others).
- Modify the `GenerateImage` method to perform a pre-flight validation check:
  * If the caller provides `crefURL` but `GetCapabilities().SupportsCref` is false, return an error: `"character reference (cref_url) is not supported by the active imagegen backend"`
  * If the caller provides a style ID that registers style references (`sref`) but `GetCapabilities().SupportsSref` is false, return an error.

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-imagegen/main.go)
- Register a new MCP tool `imagegen_get_capabilities`.
- Implement `handleGetCapabilities` to parse requests, fetch active capabilities from `ImageGenService.GetCapabilities()`, and return them as a JSON-encoded string.

---

## Verification Plan

### Automated Tests
- Add unit tests in `imagegen_test.go` checking that `GetCapabilities` returns the correct flags for different configured backends.
- Verify that calling `GenerateImage` with `crefURL` on a non-supported backend returns the validation error.
- Verify package test coverage meets the strict 91% threshold (`make check-coverage`).

### Manual Verification
- Start the `pw-mcp-imagegen` server locally.
- Use an MCP client tool inspector or test script to invoke `imagegen_get_capabilities` and check the JSON payload.
- Call `imagegen_generate` with `cref_url` when `backend = "openai"` is configured, and verify that the call fails with a validation error.
