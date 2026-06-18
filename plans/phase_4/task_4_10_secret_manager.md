# plan: Task 4.10: GCP Secret Manager Integration (`pw-mcp-cloud`)

**Status:** Open
**Go Version:** 1.26.4
**Unit Test Coverage Requirement:** 91.00%

This task implements Secret Manager integration within `pw-mcp-cloud` to allow client applications and deployment pipelines to manage and retrieve environment secrets securely.

## User Review Required

> [!CAUTION]
> Tools returning secret payloads must never write secret values to logs or console stdout unless explicitly requested by the MCP tool response schema. Telemetry components must scrub any secret values.

---

## Proposed Changes

### Secret Manager Client

Implement Secret Manager operations using `cloud.google.com/go/secretmanager/apiv1`.

#### [NEW] [secrets.go](file://../../internal/plugins/cloud/secrets.go)
- Initialize the Google Secret Manager API client.
- Implement methods:
  * `GetSecret(ctx context.Context, name string) (string, error)` (fetches the latest or specified secret version payload)
  * `CreateSecret(ctx context.Context, name, value string) (string, error)` (creates or updates a secret and adds a version payload)

#### [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- Register the new MCP tools:
  * **`cloud_get_secret`**
    * Parameters: `secret_name` (string).
  * **`cloud_create_secret`**
    * Parameters: `secret_name` (string), `secret_value` (string).
- Map MCP inputs to `GetSecret` and `CreateSecret`.

#### [NEW] [secrets_test.go](file://../../internal/plugins/cloud/secrets_test.go)
- Add unit tests verifying parameter checks, mock API client routing, and secure handling/scrubbing of secret values.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/cloud/...` to verify secret management schemas and execution pathways.
- Ensure overall coverage is maintained above 91.00%.
