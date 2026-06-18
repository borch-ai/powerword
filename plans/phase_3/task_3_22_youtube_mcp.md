# plan: Task 3.22: YouTube Integration Plugin (`pw-mcp-youtube`)

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

## Goal Description

Build a native Go Model Context Protocol (MCP) server `pw-mcp-youtube` to abstract YouTube API integration. This exposes tool interfaces for chunked video uploads, metadata/chapter updates, and analytics queries to allow Kiln and Aeolian to interact with YouTube without embedding Google API client library dependencies directly.

## User Review Required

> [!NOTE]
> This is a new native Go MCP server module. It relies on standard YouTube v3 APIs.

---

## Proposed Changes

### Sibling: Powerword (`cmd/pw-mcp-youtube`)

#### [NEW] [main.go](file://../../cmd/pw-mcp-youtube/main.go)
* Initializes the MCP server using `github.com/modelcontextprotocol/go-sdk`.
* Mounts tools:
  * `youtube_upload_video`:
    * Arguments: `video_path`, `title`, `description`, `privacy` ("public", "unlisted", "private").
    * Implementation: Uses OAuth2 credentials to perform standard chunked media upload via `google.golang.org/api/youtube/v3`. Returns the created YouTube Video ID.
  * `youtube_update_metadata`:
    * Arguments: `video_id`, `title`, `description`, `tags`, `playlist_id`.
    * Implementation: Overwrites video snippets and adds them to a target playlist.
  * `youtube_get_metrics`:
    * Arguments: `video_id`, `metrics` (slice of metrics, e.g. "views", "averageViewDuration").
    * Implementation: Queries YouTube Reporting/Analytics API and parses view totals.

---

## Verification Plan

### Automated Tests
* Create unit tests in `cmd/pw-mcp-youtube/main_test.go` asserting JSON schemas are valid and check parameters are parsed correctly from incoming MCP stdio tool calls.

### Manual Verification
1. Build the binary locally:
   ```bash
   go build -o bin/pw-mcp-youtube cmd/pw-mcp-youtube/main.go
   ```
2. Interact with the server via stdio:
   ```bash
   echo '{"method":"tools/call","params":{"name":"youtube_get_metrics","arguments":{"video_id":"123"}}}' | ./bin/pw-mcp-youtube
   ```
3. Assert it returns a graceful error indicating missing OAuth credentials or correct API payloads.
