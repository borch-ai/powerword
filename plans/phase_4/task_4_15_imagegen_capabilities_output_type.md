# plan: Task 4.15: Imagegen Capabilities Output Type Field

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-20
**Unit Test Coverage:** 91.0%

This task updates the `pw-mcp-imagegen` server to include a new capability field (`output_type`) indicating whether a backend generates still images or videos. This allows client applications like Pithos to dynamically select the correct backend model depending on the media requirements of the asset (e.g. still character seeds vs video pages).

## User Review Required

> [!NOTE]
> None.

## Proposed Changes

### Capabilities Definition

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

- Define a new `OutputType` type and constants:

  ```go
  type OutputType string

  const (
      OutputTypeImage OutputType = "image"
      OutputTypeVideo OutputType = "video"
  )
  ```

- Extend the `Capabilities` struct:

  ```go
  type Capabilities struct {
      Backend      string     `json:"backend"`
      SupportsCref bool       `json:"supports_cref"`
      SupportsSref bool       `json:"supports_sref"`
      OutputType   OutputType `json:"output_type"`
  }
  ```

- Update individual backend capabilities:
  - `OpenAIBackend.Capabilities()`: returns `OutputTypeImage`.
  - `GoogleBackend.Capabilities()`: returns `OutputTypeImage`.
  - `MidjourneyBackend.Capabilities()`: returns `OutputTypeImage`.
  - `VeoBackend.Capabilities()`: returns `OutputTypeVideo`.

### Tests

#### [MODIFY] [imagegen_test.go](file://../../internal/plugins/imagegen/imagegen_test.go)

- Update capability assertions in `TestBackendCapabilities` and `TestGetCapabilities` to verify that `output_type` matches the expected value for each backend.

#### [MODIFY] [main_integration_test.go](file://../../cmd/pw-mcp-imagegen/main_integration_test.go)

- Assert that the JSON responses returned by the `imagegen_get_capabilities` tool over the MCP channel include the correct `"output_type"` field.

---

## Verification Plan

### Automated Tests

- Run unit and integration tests and verify statement coverage remains $\ge 91\%$:

  ```bash
  make check-coverage
  ```

- Run the linter:

  ```bash
  make lint
  ```

### Manual Verification

- Start the `pw-mcp-imagegen` server.
- Invoke the `imagegen_get_capabilities` tool with `backend = "google-veo"` and verify the response JSON includes `"output_type": "video"`.
- Invoke the tool with `backend = "imagen"` and verify the response JSON includes `"output_type": "image"`.
