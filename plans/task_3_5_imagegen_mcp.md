# plan: Task 3.5: Creative Asset Generation Plugin (pw-mcp-imagegen)

**Status:** Open (Issue #TBD)

This task implements a native Go-based MCP server (`pw-mcp-imagegen`) that integrates with OpenAI (DALL-E 3) and Midjourney (via external API wrappers) to generate, catalog, and manage consistent styling references for book parodies.

## User Review Required

> [!IMPORTANT]
> Midjourney has no official REST API. We will implement an interface that supports both OpenAI's DALL-E 3 API (via standard SDK) and a configurable custom webhook/polling endpoint for third-party Midjourney automation clients (e.g., ImagineAPI or SlashAPI).

## Proposed Changes

### ImageGen Plugin Component
Create a new directory `internal/plugins/imagegen/` to contain the image generator implementation.

#### [NEW] [imagegen.go](file:///Users/human/code/powerword/internal/plugins/imagegen/imagegen.go)
- Define standard interfaces for image generation backends.
- Implement OpenAI client integration for DALL-E 3.
- Implement HTTP polling helper for Midjourney API wrappers.
- Expose the following MCP tools:
  - `imagegen_generate`: Triggers an image generation based on a text prompt, size, and style reference ID.
  - `imagegen_register_style`: Saves a visual style definition (prompt seeds or `--sref` URLs) under a semantic ID.
  - `imagegen_list_styles`: Lists currently registered style profiles.

#### [NEW] [imagegen_test.go](file:///Users/human/code/powerword/internal/plugins/imagegen/imagegen_test.go)
- Unit tests mocking API endpoints using `httptest.NewServer` to verify parameter formatting, style parsing, and file download flows.

### CLI Manifest Integration
#### [MODIFY] [internal/config/config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Register the `pw-mcp-imagegen` server within the global native plugin registry under the config key `[plugins.imagegen]`.

---

## Verification Plan

### Automated Tests
- Run `go test ./internal/plugins/imagegen/...` to assert proper error handling, mock HTTP responses, and configuration parsing.
- Enforce the 91% unit test coverage requirement.

### Manual Verification
- Set up an OpenAI API key in `powerword.toml`.
- Run `powerword "generate a 1024x1024 vintage oil painting image of a computer in a forest"` and check that the file is downloaded to the target directory.
