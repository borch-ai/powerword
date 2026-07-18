# plan: Task 3.23: Digital Music Distribution Helper (`pw-mcp-music`)

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

## Goal Description

Build a native Go Model Context Protocol (MCP) server `pw-mcp-music` to encapsulate digital music distribution assets packaging and delivery logic. Exposes tools to validate release graphics and submit metadata and audio tracks to distributor endpoints.

## User Review Required

> [!NOTE]
> This is a new native Go MCP server module. It relies on standard distribution webhook APIs.

---

## Proposed Changes

### Sibling: Powerword (`cmd/pw-mcp-music`)

#### [NEW] [main.go](file://../../cmd/pw-mcp-music/main.go)

* Initializes the MCP server using `github.com/modelcontextprotocol/go-sdk`.
* Secret Management:
  * Sourced via environment variables (e.g., `POWERWORD_MUSIC_DISTRIBUTOR_TOKEN`) rather than passed as tool arguments, to prevent sensitive tokens from leaking in LLM traces or agent transcripts.
* Mounts tools:
  * `music_validate_artwork`:
    * Arguments: `image_path`.
    * Implementation: Uses Go native `image` package to verify image resolution (must be exactly 3000x3000px) and rejects CMYK color space where detectable (e.g., JPEG decoding to `color.CMYK`), with documented limitations.
  * `music_submit_release`:
    * Arguments: `audio_path` (lossless WAV), `cover_path`, `title`, `artist`, `distributor_api_url`.
    * Implementation: Sourced `distributor_token` from environment variables/configuration, packages files as multipart form data, and posts to the distributor API endpoint, handling payload serialization.

---

## Verification Plan

### Automated Tests

* Create unit tests in `cmd/pw-mcp-music/main_test.go` to check:
  * Resolution validation correctly catches non-3000x3000px files and logs formatting violations.
  * Correct construction of the multipart form request payloads.

### Manual Verification

1. Run local build:

   ```bash
   go build -o bin/pw-mcp-music cmd/pw-mcp-music/main.go
   ```

2. Call `music_validate_artwork` with a non-compliant image size and verify that the tool returns a validation error detailing size mismatch.
