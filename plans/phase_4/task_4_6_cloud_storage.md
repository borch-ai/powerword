# plan: Task 4.6: Cloud Storage Subsystem

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-17
**Unit Test Coverage:** 91.00% (overall codebase)

This task implements a cloud storage upload capability within the native `pw-mcp-cloud` MCP server. Rather than importing a library directly, downstream services (such as Pithos and Kiln) will invoke the `cloud_upload_file` MCP tool to upload local assets and retrieve public URLs.

## User Review Required

> [!IMPORTANT]
> **Credential and Config Handling:** The `pw-mcp-cloud` server loads credential configurations dynamically using the `[plugins.cloud]` configuration block in `powerword.toml`. If the uploader is not configured or the credentials path does not exist, the server will fallback to a `NoOpUploader` uploader, failing with a clear error only when a client attempts to call `cloud_upload_file`.

---

## Proposed Changes

### Storage Uploader Engine

Create a decoupled uploader engine within the `internal/plugins/cloud` package:

#### [NEW] [uploader.go](file://../../internal/plugins/cloud/uploader.go)
- [x] Define the `Uploader` interface:
  ```go
  type Uploader interface {
      UploadFile(ctx context.Context, localPath string) (string, error)
  }
  ```
- [x] Implement `GoogleStorageUploader` which leverages GCS client libraries to upload files and set ACLs to public-read.
- [x] Implement `S3Uploader` using the AWS SDK `PutObject` call to upload objects with public-read canned ACL.
- [x] Implement `NoOpUploader` which returns an error on upload attempts (used when storage configuration is missing or disabled).

### MCP Server Integration

#### [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- [x] Initialize the `Uploader` engine on server startup using the `CloudConfig` settings.
- [x] Expose the new MCP tool:
  * **`cloud_upload_file`**
  * **Input Schema:**
    ```json
    {
      "type": "object",
      "properties": {
        "local_path": {
          "type": "string",
          "description": "The absolute path of the local file to upload."
        }
      },
      "required": ["local_path"]
    }
    ```
  * **Functionality:** Calls the configured `Uploader.UploadFile` method and returns the resulting public URL.

#### [MODIFY] [cloud_test.go](file://../../internal/plugins/cloud/cloud_test.go)
- [x] Add unit tests verifying:
  * Proper routing and instantiation of GCP, AWS, and NoOp uploaders based on configuration.
  * Correct parameter mapping and response structures for the `cloud_upload_file` tool using mock uploaders.

---

## Verification Plan

### Automated Tests
- [x] Run `go test ./internal/plugins/cloud/...` to verify the new tool schema and mock uploader execution.
- [x] Ensure package test coverage remains at or above the 91% requirement (actual coverage: 91.82%).

### Manual Verification

#### Step 1: Standalone JSON-RPC Verification
1. Compile the binary:
   ```bash
   go build -o ./bin/pw-mcp-cloud ./cmd/pw-mcp-cloud
   ```
2. Query the tool registry by piping standard MCP initialization and tools list requests:
   ```bash
   (echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'; echo '{"jsonrpc":"2.0","method":"tools/list","id":2}'; sleep 2) | ./bin/pw-mcp-cloud
   ```
   Verify that `cloud_upload_file` is successfully listed under the tools array.

3. Call the tool to trigger the default `noop` uploader fallback pathway:
   ```bash
   (echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'; echo '{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"cloud_upload_file","arguments":{"local_path":"/tmp/nonexistent.txt"}}}'; sleep 2) | ./bin/pw-mcp-cloud
   ```
   Assert that it returns a structured JSON-RPC error containing `"failed to upload file: cloud storage uploader is not configured (provider is set to 'noop')"`.

#### Step 2: Local Mock Server Verification (S3/GCS paths)
1. Launch a local mock HTTP server in a separate terminal to receive PUT/POST upload requests:
   ```bash
   python3 -c '
   import http.server
   class MyHandler(http.server.BaseHTTPRequestHandler):
       def do_PUT(self):
           print("Received PUT request at path:", self.path)
           self.send_response(200)
           self.end_headers()
   http.server.HTTPServer(("127.0.0.1", 9999), MyHandler).serve_forever()
   '
   ```
2. Trigger an upload using the mock endpoint configuration pointing to the local server:
   ```bash
   export POWERWORD_CLOUD_MOCK_ENDPOINT="http://127.0.0.1:9999"
   export POWERWORD_CLOUD_PROVIDER="s3"
   export POWERWORD_CLOUD_BUCKET="test-bucket"
   
   (echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'; echo '{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"cloud_upload_file","arguments":{"local_path":"./powerword.toml"}}}'; sleep 2) | ./bin/pw-mcp-cloud
   ```
3. Verify that the Python server prints the incoming PUT request and `pw-mcp-cloud` outputs the formatted mock public URL:
   `"http://127.0.0.1:9999/test-bucket/uploads/<token>-powerword.toml"`
