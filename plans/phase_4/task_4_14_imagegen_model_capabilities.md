# plan: Task 4.14: Model-Level Imagegen Capabilities Mapping & Overrides

**Status:** Completed
**Date Completed:** 2026-06-20
**Go Version:** 1.26.4
**Unit Test Coverage:** 91.2% (Actual)

This task updates the `pw-mcp-imagegen` server to expose per-backend capability flags and implements
manual configuration overrides so capability checks can be bypassed if upstream models change.

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

#### [MODIFY] [config.go](file:///Users/human/code/powerword/pkg/config/config.go)
- Add new properties to `ImageGenConfig` struct:
  ```go
  type ImageGenConfig struct {
      // ...
      ForceCref bool `yaml:"force_cref" mapstructure:"force_cref"`
      ForceSref bool `yaml:"force_sref" mapstructure:"force_sref"`
  }
  ```
- Configure environment binding fallbacks in configuration loader.

### Capabilities Service

#### [MODIFY] [imagegen.go](file:///Users/human/code/powerword/internal/plugins/imagegen/imagegen.go)
- Refactor `GetCapabilities()` to check both the active model name and the force override parameters.
- For `google`/`imagen` backends:
  * Parse model name (`s.cfg.Plugins.ImageGen.GoogleModel`).
  * Set `supportsCref = true` if the model matches `"veo"` or `"google-veo"`.
  * Set `supportsCref = false` if the model is an Imagen model (like `imagen-4.0-generate-001`), as Imagen does not natively support style-reference style character reference images.
- If `ForceCref` or `ForceSref` is enabled in configuration, force these values to `true` in capabilities output.
- Update `GenerateImage` to return validation errors if `cref` is requested on the Imagen backend without `ForceCref` enabled (removing the warning / text prepend fallback).

---

## Verification Plan

### Automated Tests
- Update unit tests in `imagegen_test.go` to verify capabilities mapping for Google Imagen vs Google Veo models.
- Verify overrides work as expected.
- Run `make check-coverage` to verify the coverage threshold is met.

### Manual Verification
- Query `imagegen_get_capabilities` on the running MCP server with `google` backend and `imagen-4.0-generate-001` configured. Verify it returns `supports_cref: false`.
- Call `imagegen_generate` with `cref_url` and verify it returns a validation error.
