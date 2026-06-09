# Task 5.2: GitHub Webhook Listener & Active Session Event Broker

**Status:** Completed (Issue #25)


Extend the HTTP listener daemon to handle webhook notifications for issue edits, state transitions, and PR comments, routing them dynamically as reactive MCP events.

## User Review Required

> [!WARNING]
> **Webhook Signature Verification**:
> We must strictly validate all incoming payloads using the `X-Hub-Signature-256` header against a configured webhook secret to prevent unauthorized command injection or task spoofing.

> [!IMPORTANT]
> **Port Binding Security**:
> By default, the listener daemon will only bind to `127.0.0.1` to prevent exposure on external interfaces, requiring a secure tunnel (e.g. tailscale, ngrok) for actual GitHub callback delivery.

## Proposed Changes

### Webhook Component

#### [NEW] [listener.go](../internal/review/listener.go)
- Create HTTP handler to parse and validate GitHub webhook payloads for:
  - `issues` (triggered when plans are edited or closed).
  - `issue_comment` (triggered when users approve plans or comment on progress).
  - `pull_request_review_comment` (triggered when reviewer bots leave reviews).
- Trigger local event broker handlers upon validation.

#### [NEW] [mcp.go](../internal/review/mcp.go)
- Integrate with MCP server interface to expose webhook events as standard MCP notifications.
- Expose resource schemas representing the active issues and PR reviews to the model context.

### CLI Bindings

#### [MODIFY] [config.go](../internal/config/config.go)
- Add webhook secret (`WebhookSecret`) and port (`WebhookPort`) configurations to Viper bindings, mapping to `POWERWORD_WEBHOOK_SECRET` and `POWERWORD_WEBHOOK_PORT`.

#### [MODIFY] [review.go](../cmd/powerword/review.go)
- Add `--listen` and `--port` flags to the `review` command. If `--listen` is provided, start the HTTP webhook listener.

---

## Verification Plan

### Automated Tests
- Run command: `go test ./internal/review/...`
- Unit tests verifying:
  - HMAC SHA256 signature verification logic.
  - Parsing of `issues`, `issue_comment`, and `pull_request_review_comment` payloads.
  - Correct dispatch of events to the internal MCP Server (`WebhookMCPServer`).
  - Resource schema exposure under `github://issues`.
- Final Go version used: Go 1.26.4

### Manual Verification
- Launch the listener local server: `powerword review --listen --port 8080`.
- Send a mock signed issue comment payload using `curl` and verify it logs event dispatching.

