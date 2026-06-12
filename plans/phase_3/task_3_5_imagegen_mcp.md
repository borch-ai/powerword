# plan: Task 3.5: Creative Asset Generation Plugin (pw-mcp-imagegen)

**Status:** Completed
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%
**Completed At:** 2026-06-11T02:23:00Z
**Go Version:** 1.26.4

This task implements a native Go-based MCP server (`pw-mcp-imagegen`) that integrates with OpenAI (DALL-E 3), Google AI Studio (Imagen 3), and Midjourney (via external API wrappers) to generate, catalog, and manage consistent styling references for book parodies.

## User Review Required

> [!IMPORTANT]
> Midjourney has no official REST API. We will implement an interface that supports both OpenAI's DALL-E 3 API (via standard SDK), Google AI Studio's Imagen 3 API, and a configurable custom webhook/polling endpoint for third-party Midjourney automation clients (e.g., ImagineAPI or SlashAPI).

## Proposed Changes

### ImageGen Plugin Component
Create a new directory `internal/plugins/imagegen/` to contain the image generator implementation.

#### [NEW] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)
- [x] Define standard interfaces for image generation backends.
- [x] Implement OpenAI client integration for DALL-E 3.
- [x] Implement Google predict client integration for Imagen 3 (`imagen-3.0-generate-002`).
- [x] Implement Google Veo video generation support (`veo-2.0-generate-001`) via long-running operation polling.
- [x] Implement HTTP polling helper for Midjourney API wrappers.
- [x] Expose the following MCP tools:
  - `imagegen_generate`: Triggers an image generation based on a text prompt, size, and style reference ID.
  - `imagegen_register_style`: Saves a visual style definition (prompt seeds or `--sref` URLs) under a semantic ID.
  - `imagegen_list_styles`: Lists currently registered style profiles.

#### [NEW] [imagegen_test.go](file://../../internal/plugins/imagegen/imagegen_test.go)
- [x] Unit tests mocking API endpoints using `httptest.NewServer` to verify parameter formatting, style parsing, and file download flows.

### CLI Manifest Integration
#### [MODIFY] [config.go](file://../../pkg/config/config.go)
- [x] Register the `pw-mcp-imagegen` server within the global native plugin registry under the config key `[plugins.imagegen]`.

### Verification and Test Scripts
#### [MODIFY] [check_coverage.go](file://../../cmd/powerword/check_coverage.go)
- [x] Clean the coverage profile of concurrency-generated null bytes and execute the `go tool cover` tool internally.
#### [MODIFY] [Makefile](file://../../Makefile)
- [x] Pass `coverage.out` directly to the updated `check_coverage.go` validator script.

---

## Verification Plan

### Automated Tests
- [x] Run `make test` and `make check-coverage` to assert proper error handling, mock HTTP responses, and configuration parsing.
- [x] Enforce the 91% unit test coverage requirement (achieved **91.10%**).

### Manual Verification
- [x] Set up an OpenAI or Google Gemini API key in `powerword.toml` or environment variables:
  ```toml
  [api_keys]
  gemini = "YOUR_GEMINI_API_KEY"
  openai = "YOUR_OPENAI_API_KEY"
  ```
- [x] Validate Image Generation (OpenAI / Imagen 3):
  1. Configure `backend = "google"` or `backend = "openai"` in `powerword.toml`.
  2. Invoke CLI tool/prompt: `powerword "generate a 1024x1024 vintage oil painting image of a computer in a forest"`.
  3. Verify that the image file is successfully downloaded to the `generated_images/` directory in the workspace.
  4. Inspect the image extension and content to ensure correct visual representation and slugified naming schema (e.g. `image_1717830000_generate-a-1024x1024-vintage-oil.png`).
- [x] Validate Creative Video Generation (Veo):
  1. Set `backend = "veo"` and `google_model = "veo-2.0-generate-001"` in `powerword.toml`.
  2. Run the generator tool: `powerword "generate a 5 second video of a red panda playing in the snow"`.
  3. Verify that the client polls Google's long-running operation endpoint until done.
  4. Verify that the final `.mp4` video file is downloaded to the `generated_images/` directory.
- [x] Validate Consistent Styling References:
  1. Call MCP tool `imagegen_register_style` with `style_id = "steampunk"`, `prompt_seed = "highly detailed brass gears, copper pipes, steam vents"`.
  2. Run generation using that style ID: `powerword "generate a locomotive --style steampunk"`.
  3. Verify that the style store loads `.powerword/imagegen_styles.json` and prepends the style details to the prompt correctly, producing the styled output.
