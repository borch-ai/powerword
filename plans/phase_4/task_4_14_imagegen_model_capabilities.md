# plan: Task 4.14: Backend-Owned Imagegen Capabilities Mapping & Overrides

**Status:** Completed
**Date Completed:** 2026-06-20
**Go Version:** 1.26.4
**Unit Test Coverage:** 91.2% (Actual)
**PR:** [#115](https://github.com/borch-ai/powerword/pull/115)

This task updates the `pw-mcp-imagegen` server to expose per-backend capability flags via a
backend-owned `Capabilities()` method, and implements operator-configurable overrides so capability
checks can be bypassed if upstream models change.

## Implementation Notes

> [!NOTE]
> This task is complete and the PR is open awaiting merge approval.
> Copilot reviewed the PR across two commits; all 7 comments were addressed.

## Final Implementation

**Architecture: Backend-Owned Capabilities**

Each backend struct owns and reports its own capability set via a `Capabilities()` method.
The service delegates to the active backend type (via a zero-value instance), then applies
`ForceCref`/`ForceSref` configuration overrides:

| Backend | `supports_cref` | `supports_sref` |
|---|---|---|
| `OpenAIBackend` | `false` | `false` |
| `GoogleBackend` (Imagen API) | `false` | `false` |
| `VeoBackend` | `true` | `false` |
| `MidjourneyBackend` | `true` | `true` |

`ImageGenService.GetCapabilities()` delegates to the active backend type then applies
`ForceCref`/`ForceSref` overrides from config/env vars.

The legacy cref text-prepend fallback in `GoogleBackend.GenerateImage()` was removed. If `cref_url`
is provided on the Imagen backend without `ForceCref`, `GenerateImage` returns a validation error.

## Proposed Changes

### Configuration Layer

#### [MODIFY] [config.go](../../pkg/config/config.go)
- Add `ForceCref` and `ForceSref` fields to `ImageGenConfig` struct:
  ```go
  ForceCref bool `mapstructure:"force_cref"`
  ForceSref bool `mapstructure:"force_sref"`
  ```
- Add `SetDefault` calls and `bindEnv` bindings for `POWERWORD_IMAGEGEN_FORCE_CREF` / `POWERWORD_IMAGEGEN_FORCE_SREF`.

### Capabilities Service

#### [MODIFY] [imagegen.go](../../internal/plugins/imagegen/imagegen.go)
- Add `Capabilities()` method to `OpenAIBackend`, `GoogleBackend`, `VeoBackend`, `MidjourneyBackend`.
- Refactor `GetCapabilities()` on `ImageGenService` to delegate to the active backend type (via
  zero-value struct instance), then apply `ForceCref`/`ForceSref` overrides.
- Remove legacy cref text-prepend fallback from `GoogleBackend.GenerateImage()`; add accurate
  inline comment clarifying when the cref path is reached.
- `GenerateImage` returns a validation error if `cref_url` is provided without `ForceCref` on an
  Imagen backend.

### Tests

#### [MODIFY] [imagegen_test.go](../../internal/plugins/imagegen/imagegen_test.go)
- `TestBackendCapabilities`: table-driven test verifying each backend's `Capabilities()` directly.
- `TestGetCapabilities`: updated table (removed model-name cases; google/imagen always `false`).
- `TestGetCapabilities_ForceOverrides`: verifies ForceCref/ForceSref override behavior (3 sub-tests).
- `TestGenerateImage_ImagenCrefValidation`: hermetic httptest-backed test; verifies Imagen rejects
  cref and ForceCref bypasses it.
- `TestGoogleBackend_CharacterReference`: updated to expect plain prompt (no text prepend).

#### [MODIFY] [main_integration_test.go](../../cmd/pw-mcp-imagegen/main_integration_test.go)
- `TestMCP_ImageGenPlugin_GoogleBackend_CrefNotSupported`: verifies `supports_cref: false` over MCP
  and that `cref_url` returns a capability validation error for the google/Imagen backend.
- `TestMCP_ImageGenPlugin_ForceCref_BypassesValidation`: hermetic test with mock Imagen server;
  verifies `force_cref = true` exposes `supports_cref: true` and allows the generate call to succeed.

---

## Verification Plan

### Automated Tests
All tests hermetic (httptest mocks, no real network calls).

- `make check-coverage` → **91.2%** ✅ (threshold: 91.0%)
- `make lint` → **0 issues** ✅
- Unit tests: `go test ./internal/plugins/imagegen/...` → **PASS** ✅
- Integration tests: `go test -tags=integration ./cmd/pw-mcp-imagegen/...` → **PASS** ✅

### PR & Code Review
- PR [#115](https://github.com/borch-ai/powerword/pull/115) opened against `main`.
- Copilot Code Review posted 4 comments on commit `5e64a20`; all addressed in follow-up commit `7563c0e`.
- Copilot did not post further comments on `7563c0e` after the 7-minute polling window.

### Manual Verification
- Query `imagegen_get_capabilities` with `backend = "google"` + `google_model = "imagen-4.0-generate-001"` → `supports_cref: false`.
- Call `imagegen_generate` with `cref_url` on the google backend → capability validation error.
- Set `force_cref = true` in `powerword.toml` → `supports_cref: true`, generate call proceeds.
