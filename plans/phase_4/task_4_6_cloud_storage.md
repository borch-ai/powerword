# plan: Task 4.6: Cloud Storage Subsystem

**Status:** Open (Issue #TBD)

This task implements a cloud storage upload capability within the native `pw-mcp-cloud` MCP server. Rather than importing a library directly, downstream services (such as Pithos and Kiln) will invoke the `cloud_upload_file` MCP tool to upload local assets and retrieve public URLs.

## User Review Required

> [!IMPORTANT]
> **Credential and Config Handling:** The `pw-mcp-cloud` server loads credential configurations dynamically using the `[plugins.cloud]` configuration block in `powerword.toml`. If the uploader is not configured or the credentials path does not exist, the server will fallback to a `noop` uploader, failing with a clear error only when a client attempts to call `cloud_upload_file`.

---

## Proposed Changes

### Storage Uploader Engine

Create a decoupled uploader engine within the `internal/plugins/cloud` package:

#### [NEW] [uploader.go](file://../../internal/plugins/cloud/uploader.go)
- [ ] Define the `Uploader` interface:
  ```go
  type Uploader interface {
      UploadFile(ctx context.Context, localPath string) (string, error)
  }
  ```
- [ ] Implement `GoogleStorageUploader` which leverages GCS client libraries to upload files and set ACLs to public-read.
- [ ] Implement `S3Uploader` using the AWS SDK Transfer Manager to upload objects.
- [ ] Implement `NoOpUploader` which returns an error on upload attempts (used when storage configuration is missing or disabled).

### MCP Server Integration

#### [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- [ ] Initialize the `Uploader` engine on server startup using the `CloudConfig` settings.
- [ ] Expose the new MCP tool:
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
- [ ] Add unit tests verifying:
  * Proper routing and instantiation of GCP, AWS, and NoOp uploaders based on configuration.
  * Correct parameter mapping and response structures for the `cloud_upload_file` tool using mock uploaders.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/plugins/cloud/...` to verify the new tool schema and mock uploader execution.
- [ ] Ensure package test coverage remains at or above the 91% requirement.

### Manual Verification
- [ ] Start `pw-mcp-cloud` locally.
- [ ] Execute `cloud_upload_file` tool via `powerword` or using a manual MCP client payload, passing a local test file path. Verify that a valid public URL is returned.

