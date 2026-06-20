# plan: Task 4.8: Google Cloud Run Orchestration Tools (`pw-mcp-cloud`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-20
**Unit Test Coverage:** 91.00%

---

Extend the native `pw-mcp-cloud` MCP server to support Google Cloud Run serverless container deployments. Downstream services (such as Kiln and Lamplighter) can invoke these tools to list, inspect, and deploy serverless containers.

## User Review Required

> [!IMPORTANT]
> **Authentication & Credentials:**
> Cloud Run management requires active GCP permissions (e.g. Cloud Run Admin). The server resolves credentials using the existing workspace configuration under `[plugins.cloud]` or standard environment variables (`GOOGLE_APPLICATION_CREDENTIALS`, `GOOGLE_CLOUD_PROJECT`).
> Regional endpoints (`https://{region}-run.googleapis.com`) will be used to correctly route requests to targeted deployment regions.

---

## Proposed Changes

### Cloud Plugin Package

#### [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- Define `CloudRunService` metadata response struct.
- Define `GCPRunClient` interface for client decoupling and unit test mocking.
- Implement `realGCPRunClient` that instantiates `google.golang.org/api/run/v1` client and wraps the regional API calls.
- Add `GCPRunClient` to `CloudService` struct and wire it up in `NewCloudService`.
- Add service methods to `CloudService`:
  - `ListRunServices(ctx context.Context, region string) ([]CloudRunService, error)`
  - `GetRunService(ctx context.Context, region, serviceName string) (*CloudRunService, error)`
  - `DeployRunService(ctx context.Context, region, serviceName, image string, envVars map[string]string, concurrency int64, cpu, memory string) (*CloudRunService, error)`

#### [MODIFY] [cloud_test.go](file://../../internal/plugins/cloud/cloud_test.go)
- Implement `mockGCPRunClient` for unit testing.
- Add unit tests for list, get, and deploy Cloud Run operations.
- Expand `TestRealClientsWithMockHTTP` to cover mock JSON HTTP requests for Cloud Run APIs.

### MCP Server Command

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-cloud/main.go)
- Add schemas for the new tools:
  - `cloud_deploy_run_service`
  - `cloud_get_run_service`
  - `cloud_list_run_services`
- Register these tools in `setupServer`.
- Implement JSON parameter parsing and execution handlers for each tool.

---

## Verification Plan

### Automated Tests
- Run unit tests to check mock behaviors and client configurations:
  ```bash
  go test -v ./internal/plugins/cloud/...
  ```
- Run linter and verify 91% code coverage target:
  ```bash
  make lint
  make check-coverage
  ```

### Manual Verification
- Compile the updated cloud server binary:
  ```bash
  go build -o ./bin/pw-mcp-cloud ./cmd/pw-mcp-cloud
  ```
- Verify that the tools are successfully listed in the MCP initialization output:
  ```bash
  (echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'; echo '{"jsonrpc":"2.0","method":"tools/list","id":2}'; sleep 1) | ./bin/pw-mcp-cloud
  ```
