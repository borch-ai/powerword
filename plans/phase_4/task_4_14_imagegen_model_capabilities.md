# plan: Task 4.14: Model-Level Imagegen Capabilities Mapping & Overrides

**Status:** Completed
**Date Completed:** 2026-06-20
**Go Version:** 1.26.4
**Unit Test Coverage:** 91.2% (Actual)

This task updates the `pw-mcp-imagegen` server to expose per-backend capability flags and implements
manual configuration overrides so capability checks can be bypassed if upstream models change.

## User Review Required

> [!NOTE]
> This task is complete. No further review is required. Notes below describe the final choices made.

## Final Implementation

**Architecture: Backend-Owned Capabilities**

Instead of the service doing model-name string matching, each backend struct owns its own capabilities
via a `Capabilities()` method:
- `OpenAIBackend.Capabilities()` → `{supports_cref: false, supports_sref: false}`
- `GoogleBackend.Capabilities()` → `{supports_cref: false, supports_sref: false}` (Imagen never supports cref)
- `VeoBackend.Capabilities()` → `{supports_cref: true, supports_sref: false}`
- `MidjourneyBackend.Capabilities()` → `{supports_cref: true, supports_sref: true}`

`ImageGenService.GetCapabilities()` delegates to the active backend type (via a zero-value instance),
then applies `ForceCref`/`ForceSref` overrides from configuration.

The legacy cref text-prepend fallback in `GoogleBackend.GenerateImage()` was removed. If `cref_url` is
provided on the Imagen backend without `ForceCref`, `GenerateImage` returns a validation error.

## Proposed Changes

### Configuration Layer

#### [MODIFY] [config.go](../../pkg/config/config.go)
- Add new properties to `ImageGenConfig` struct:
  ```go
  type ImageGenConfig struct {
      // ...
      ForceCref bool `yaml:"force_cref" mapstructure:"force_cref"`
      ForceSref bool `yaml:"force_sref" mapstructure:"force_sref"`
  }
  ```
- Configure environment binding fallbacks (`POWERWORD_IMAGEGEN_FORCE_CREF`, `POWERWORD_IMAGEGEN_FORCE_SREF`) in configuration loader.

### Capabilities Service

#### [MODIFY] [imagegen.go](../../internal/plugins/imagegen/imagegen.go)
- Add `Capabilities()` method to each backend struct (`OpenAIBackend`, `GoogleBackend`, `VeoBackend`, `MidjourneyBackend`).
- Refactor `GetCapabilities()` on `ImageGenService` to delegate to the active backend type, then apply force overrides.
- Remove legacy cref text-prepend fallback from `GoogleBackend.GenerateImage()`.
- `GenerateImage` returns a validation error if `cref_url` is provided on the Imagen backend without `ForceCref` enabled.

---

## Verification Plan

### Automated Tests
- `TestBackendCapabilities`: table-driven test verifying each backend's `Capabilities()` method directly.
- `TestGetCapabilities`: updated to cover backend-owned model (google/imagen always `false` for cref).
- `TestGetCapabilities_ForceOverrides`: verifies `ForceCref`/`ForceSref` override behavior.
- `TestGenerateImage_ImagenCrefValidation`: verifies Imagen backend rejects cref; ForceCref bypasses it.
- Integration: `TestMCP_ImageGenPlugin_GoogleBackend_CrefNotSupported` and `TestMCP_ImageGenPlugin_ForceCref_BypassesValidation`.
- Coverage achieved: **91.2%** (threshold: 91.0%).

### Manual Verification
- Query `imagegen_get_capabilities` on the running MCP server with `google` backend and `imagen-4.0-generate-001` configured. Verify it returns `supports_cref: false`.
- Call `imagegen_generate` with `cref_url` and verify it returns a validation error.
- Set `force_cref = true` in config and verify `supports_cref` becomes `true`.
