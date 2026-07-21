# plan: Task 4.11: IAM Policies & Pre-flight API Verification (`pw-mcp-cloud`)

**Status:** Open
**Go Version:** 1.26.4
**Unit Test Coverage Requirement:** 91.00%

This task implements pre-flight validation tools to verify GCP IAM credentials, permissions, and service usage API statuses before attempting deployments.

## User Review Required

> [!IMPORTANT]
> Performing pre-flight checks helps identify permission gaps (such as missing `run.admin` or `secretmanager.secretAccessor` roles) and disabled APIs early, avoiding opaque errors during deployments.

---

## Proposed Changes

### Pre-flight Verification Client

Implement API and IAM validation modules using `google.golang.org/api/serviceusage/v1` and `cloud.google.com/go/iam`.

#### [NEW] [verify.go](file://../../internal/plugins/cloud/verify.go)

- Initialize Service Usage and Cloud Resource Manager API clients.
- Implement methods:
  - `VerifyIAMRoles(ctx context.Context, projectID, member string, roles []string) (map[string]bool, error)`
  - `CheckEnabledAPIs(ctx context.Context, projectID string, apis []string) (map[string]bool, error)`

#### [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)

- Register the new MCP tools:
  - **`cloud_verify_iam_roles`**
    - Parameters: `project_id` (string), `member` (string, e.g. `serviceAccount:deployer@project.iam.gserviceaccount.com`), `roles` (array of strings).
  - **`cloud_check_enabled_apis`**
    - Parameters: `project_id` (string), `apis` (array of strings).
- Map MCP inputs to corresponding functions in `verify.go`.

#### [NEW] [verify_test.go](file://../../internal/plugins/cloud/verify_test.go)

- Add unit tests verifying permission reporting structures and enabled API response formats using mock API clients.

---

## Verification Plan

### Automated Tests

- Run `go test -v ./internal/plugins/cloud/...` to verify new validation schemas and execution paths.
- Ensure overall coverage is maintained above 91.00%.
