# plan: Task 4.4: Cloud Orchestrator Plugin (AWS/GCP)

**Status:** Open (Issue #TBD)

This task implements a lightweight native Go-based MCP server (`pw-mcp-cloud`) that queries cloud provider APIs (AWS and GCP) to check deployment status, view metadata (EC2/GCE instance states, CloudWatch logs, storage buckets), and inspect target configurations.

## User Review Required

> [!WARNING]
> Accessing cloud resources requires credential keys. We must use standard AWS SDK (`aws-sdk-go-v2`) and Google Cloud client libraries, loading credentials securely via standard environment variables (`AWS_ACCESS_KEY_ID`, `GOOGLE_APPLICATION_CREDENTIALS`) or local config files. The agent must never expose or leak these keys in logs.

## Proposed Changes

### Cloud Plugin Component
Create a new directory `internal/plugins/cloud/` to contain the cloud orchestrator clients.

#### [NEW] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- [ ] Initialize AWS/GCP clients using standard SDK credential configuration.
- [ ] Expose the following MCP tools:
  - `cloud_list_instances`: Retrieves the state of VM instances (EC2/GCE) filtered by tags or status.
  - `cloud_get_logs`: Retrieves container or infrastructure log streams (CloudWatch/Stackdriver).
  - `cloud_check_bucket`: Verifies bucket configuration and checks basic object metadata.

#### [NEW] [cloud_test.go](file://../../internal/plugins/cloud/cloud_test.go)
- [ ] Mock AWS/GCP service client interfaces to verify parameter routing and metadata serialization without live cloud calls.

### CLI Manifest Integration
#### [MODIFY] [config.go](file://../../pkg/config/config.go)
- [ ] Register the `pw-mcp-cloud` server within the native plugin registry under the config key `[plugins.cloud]`.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/plugins/cloud/...` to assert mock client outputs and config bindings.
- [ ] Enforce the 91% unit test coverage requirement.

### Manual Verification
- [ ] Set up a sandbox AWS or GCP environment.
- [ ] Run `powerword "check deployment status of ec2 instances with tag environment=production"` and verify instance details are displayed cleanly.
