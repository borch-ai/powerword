# plan: Task 4.4: Cloud Orchestrator Plugin (AWS/GCP)

**Status:** Done

This task implements a lightweight native Go-based MCP server (`pw-mcp-cloud`) that queries cloud provider APIs (AWS and GCP) to check deployment status, view metadata (EC2/GCE instance states, CloudWatch logs, storage buckets), and inspect target configurations.

## User Review Required

> [!WARNING]
> Accessing cloud resources requires credential keys. We must use standard AWS SDK (`aws-sdk-go-v2`) and Google Cloud client libraries, loading credentials securely via standard environment variables (`AWS_ACCESS_KEY_ID`, `GOOGLE_APPLICATION_CREDENTIALS`) or local config files. The agent must never expose or leak these keys in logs.

---

## Proposed Changes

### Configuration & Manifest Integration

#### [MODIFY] [config.go](file://../../pkg/config/config.go)
- [x] Add the `CloudConfig` schema mapping to `config.Config`:
  ```go
  type CloudConfig struct {
      Provider        string `mapstructure:"provider"`         // "gcs", "s3", or "noop"/"mock"
      Bucket          string `mapstructure:"bucket"`           // storage bucket name
      CredentialsPath string `mapstructure:"credentials_path"` // path to service account JSON or AWS credentials
      Region          string `mapstructure:"region"`           // AWS Region (e.g. us-east-1)
  }
  ```
- [x] Add `Cloud CloudConfig `mapstructure:"cloud"`` field to `PluginsConfig` struct.
- [x] Set sensible defaults in `LoadConfig` for `plugins.cloud.provider` ("noop").
- [x] Register the `pw-mcp-cloud` server within the native plugin registry under the config key `[plugins.cloud]`.

### Cloud Plugin Component
Create a new directory `internal/plugins/cloud/` to contain the cloud orchestrator clients.

#### [NEW] [cloud.go](file://../../internal/plugins/cloud/cloud.go)
- [x] Initialize AWS/GCP clients using standard SDK credential configuration or paths defined in `CloudConfig`.
- [x] Expose the following MCP tools:
  - `cloud_list_instances`: Retrieves the state of VM instances (EC2/GCE) filtered by tags or status.
  - `cloud_get_logs`: Retrieves container or infrastructure log streams (CloudWatch/Stackdriver).
  - `cloud_check_bucket`: Verifies bucket configuration and checks basic object metadata.

#### [NEW] [cloud_test.go](file://../../internal/plugins/cloud/cloud_test.go)
- [x] Mock AWS/GCP service client interfaces to verify parameter routing and metadata serialization without live cloud calls.

---

## Verification Plan

### Automated Tests
- [x] Run unit tests (`go test ./internal/plugins/cloud/...`) to assert mock client outputs and config bindings.
- [x] Run stdio-based integration tests (`make test-integration`) to assert process launching and tool call responses.
- [x] Enforce the 91% unit test coverage requirement (currently at 91.0%).
- [x] Ensure Go version `1.26.4` is utilized.

### Manual Verification
- [x] Setup and check schema response manually: `./bin/pw-mcp-cloud`
