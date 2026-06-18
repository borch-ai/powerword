# plan: Task 4.9: Artifact Registry & Container Delivery Verification (`pw-mcp-cloud`)

**Status:** Open
**Go Version:** 1.26.4
**Unit Test Coverage Requirement:** 91.00%

This task implements Google Artifact Registry verification tools inside `pw-mcp-cloud` to check for container image tags and delivery statuses before attempting deployments.

## User Review Required

> [!NOTE]
> This tool enables validation of container availability in Artifact Registry repositories prior to triggering Cloud Run deployments, preventing "ImageNotReady" loops in deployment pipelines.

---

## Proposed Changes

### Artifact Registry Client

Implement Google Artifact Registry checks using `cloud.google.com/go/artifactregistry/apiv1`.

#### [NEW] [registry.go](file://../../internal/plugins/cloud/registry.go)
- Initialize the Artifact Registry API client.
- Implement `CheckRegistryImage(ctx context.Context, repository, image, tag, region string) (bool, error)` to verify if the specified tag is uploaded and readable.

#### [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- Register the new MCP tool:
  * **`cloud_check_registry_image`**
    * Parameters: `repository` (string, e.g. `projects/my-project/locations/us-central1/repositories/my-repo`), `image_name` (string), `tag` (string).
- Route the tool handler to `CheckRegistryImage` logic.

#### [NEW] [registry_test.go](file://../../internal/plugins/cloud/registry_test.go)
- Add unit tests using mock API client layers to check correct handling of image-found, image-not-found, and API-error states.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/cloud/...` to verify tool registrations and mock API responses.
- Ensure package test coverage meets the 91% requirement.
