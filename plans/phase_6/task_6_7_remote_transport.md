# plan: Task 6.7: Remote SSE & WebSocket Client Transport

**Status:** Open (Issue #TBD)

This task extends the MCP client capabilities by adding transport layers to connect to remote MCP plugins over Server-Sent Events (SSE) and WebSockets, including TLS and header authorization.

## User Review Required

> [!WARNING]
> Connecting to remote endpoints exposes the workspace to external inputs. Ensure that TLS verification is enabled by default and certificates are correctly validated.

## Proposed Changes

### MCP Transport

#### [MODIFY] [client.go](file://../../pkg/llm/client.go)

- [ ] Implement SSE and HTTP/WebSocket transport mappings matching the `modelcontextprotocol/go-sdk` standards.
- [ ] Support custom HTTP headers to pass API keys or bearer tokens.

---

## Verification Plan

### Automated Tests

- [ ] Run `go test ./internal/mcp/...` spinning up an `httptest` SSE server to verify message loops.

### Manual Verification

- [ ] Spin up an external Node/Python SSE MCP server and connect Powerword client to it via remote configuration.
