# plan: Task 3.9: Google Doc MCP Plugin (GDoc)

**Status:** Completed
**Go Version:** 1.26.5
**Date Completed:** 2026-07-13
**Unit Test Coverage:** 91.0%

Implement a native Go Model Context Protocol (MCP) server `pw-mcp-gdoc` in the Powerword repository to interact with Google Docs and Google Drive. This enables agents and pipelines (such as Pithos) to export manuscripts to Google Docs for human review and import finalized text back.

## User Review Required

> [!IMPORTANT]
> **Google API Credentials**:
> Running this plugin requires Google API credentials. We will support two authentication methods:
> 1. **OAuth2 User Auth**: A local browser-based authentication flow on first run. It will prompt the user to authorize the app via an OAuth URL, and save the access/refresh token locally to `~/.config/powerword/gdoc_token.json`.
> 2. **Service Account Key**: A path to a Google Cloud Service Account JSON key file specified in `powerword.toml`. The user must share target Google Docs or Google Drive folders with the service account email address.

---

## Proposed Changes

### Plugin Command

#### [NEW] [main.go](file://../../cmd/pw-mcp-gdoc/main.go)
- Entry point for the `pw-mcp-gdoc` binary.
- Set up MCP Server session using `modelcontextprotocol/go-sdk`.
- Register the following tools:
  - `gdoc_create`: Creates a new Google Doc with a title and content. Returns the `doc_id` and edit `url`.
  - `gdoc_read`: Reads raw text content of a Google Doc given its `doc_id`.
  - `gdoc_update`: Overwrites or modifies a Google Doc with new content.
- Initialize Google API clients (Docs API and Drive API) using `google.golang.org/api/docs/v1` and `google.golang.org/api/drive/v3`.

### Configuration Layer

#### [MODIFY] [powerword.example.toml](file://../../powerword.example.toml)
- Document configuration settings for the new `gdoc` plugin:
  ```toml
  [plugins.gdoc]
  credentials_path = "~/.config/powerword/credentials.json"
  token_path = "~/.config/powerword/gdoc_token.json"
  # Or for service account:
  # service_account_path = "~/.config/powerword/service_account.json"
  ```

---

## Verification Plan

### Automated Tests
- Run `go test -v ./cmd/pw-mcp-gdoc/...`
- Mock Google Docs/Drive API requests using custom HTTP transport clients (`http.RoundTripper` mocks) to test the tool handler outputs without making network requests.

### Manual Verification
- Compile the plugin: `go build -o bin/pw-mcp-gdoc ./cmd/pw-mcp-gdoc`.
- Run `bin/pw-mcp-gdoc` directly in a terminal using JSON-RPC stdio requests.
- Verify that `gdoc_create` creates a doc on Google Drive and returns a valid URL, and `gdoc_read` returns the document content.
