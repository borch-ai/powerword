# plan: Task 4.12: Google Veo & Imagen Character Seeding Support

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-18
**Unit Test Coverage:** 91.1%

This task implements character visual consistency for the Google Veo (video generation) and Google Imagen (image generation) backends in the `pw-mcp-imagegen` server. Currently, when `cref_url` and `character_weight` are passed to `imagegen_generate`, they are silently ignored if the backend is configured to `"google"` (Imagen) or `"veo"` (Google Veo).

To resolve this, we will extend the `GenerateImage` interfaces and payload builders for both backends:

1. **Google Veo (Video):** Pass the `cref_url` (reference image URL) as the starting image frame (image input) for image-to-video generation.
2. **Google Imagen (Image):** Support reference image injection if supported by the model (e.g. style reference or input image for image-to-image/editing), or append the character profile guidelines dynamically or log clean warnings when image references are not natively supported, ensuring a robust developer experience.

## User Review Required

> [!NOTE]
> **API Capabilities:** This task requires updating Google GenAI and Veo parameter mapping. If a specific version of the Google API does not natively support base64 encoded starting frames or subject references, a fallback option (e.g. logging a detailed warning or leveraging public GCS URIs) must be verified.

---

## Proposed Changes

### Configuration & Services

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

- Update `GenerateImage` method signatures for both `GoogleBackend` and `VeoBackend` to accept optional `crefURL` (string) and `characterWeight` (integer pointer).
- Update the switch block in `ImageGenService.GenerateImage` to pass `crefURL` and `characterWeight` to `runGoogle` and `runVeo`.
- Update `runGoogle` and `runVeo` to pass `crefURL` and `characterWeight` to their respective backend `GenerateImage` calls.
- In `GoogleBackend.GenerateImage`:
  - Accept `crefURL` and `characterWeight`.
  - If `crefURL` is provided, map it to the API payload if Google AI Studio / Vertex AI Imagen 3 supports reference-based conditioning (e.g. as a style or subject reference), or log a warning if the model does not support image-based references and fallback to prepended character description.
- In `VeoBackend.GenerateImage`:
  - Accept `crefURL` and `characterWeight`.
  - Map `crefURL` to the `instances[0].image` field (using a `gcsUri` or fetching the image bytes and encoding them as `imageBytes` base64 if needed by the model's image-to-video capabilities).

---

## Verification Plan

### Automated Tests

- Add unit tests in `imagegen_test.go` verifying that `crefURL` and `characterWeight` parameters are correctly propagated to `runVeo` and `runGoogle` and marshal correctly in their request payloads.
- Run tests and check coverage threshold: `make check-coverage`

### Manual Verification

- Configure the backend to `google` or `veo` in `powerword.toml`.
- Run `pw-mcp-imagegen` and call the `imagegen_generate` tool with a valid `cref_url` public URL.
- Inspect request payloads or debug output to verify Google GenAI endpoints receive the seed reference image.
