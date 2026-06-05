# Task 5.2: GitHub Webhook Listener & Event Broker

Build an MCP-enabled HTTP listener daemon that registers webhooks from GitHub (such as new PR comments or CI run completions) and surfaces these notifications as reactive agent events.

## User Review Required

> [!WARNING]
> **Webhook Signature Verification**:
> We must strictly validate all incoming payloads using the `X-Hub-Signature-256` header against a configured webhook secret to prevent unauthorized command injection or task spoofing.

> [!IMPORTANT]
> **Port Binding Security**:
> By default, the listener daemon will only bind to `127.0.0.1` to prevent exposure on external interfaces, requiring a secure tunnel (e.g. tailscale, ngrok) for actual GitHub callback delivery.

## Proposed Changes

### Webhook Component

#### [NEW] [listener.go](file:///Users/human/code/powerword/internal/review/listener.go)
- Create HTTP handler to parse and validate GitHub webhook payloads (`pull_request_review_comment`, `workflow_run`).
- Trigger local event broker handlers upon validation.

#### [NEW] [mcp.go](file:///Users/human/code/powerword/internal/review/mcp.go)
- Integrate with MCP server interface to expose webhook events as standard MCP notifications.

### CLI Bindings

#### [MODIFY] [config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Add webhook secret and port configurations to Viper bindings.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/review/...`
- Unit tests verifying:
  - HMAC SHA256 signature verification logic.
  - Parsing of `pull_request_review_comment` payloads.
  - Correct dispatch of events to the internal broker.

### Manual Verification
- Launch the listener local server: `powerword review --listen --port 8080`.
- Send a mock signed payload using `curl` and verify it is parsed and logs success.
