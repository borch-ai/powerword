# Task 2.1: MCP Go SDK Integration

Integrate the Model Context Protocol (MCP) Go SDK (`github.com/modelcontextprotocol/go-sdk`) into the Powerword binary. Establish client abstractions to initialize sessions, query capabilities, and translate schemas between LLM tools and MCP definitions.

## User Review Required

> [!NOTE]
> MCP client interactions will be encapsulated under the [internal/mcp](file:///Users/human/code/powerword/internal/mcp) package.

## Proposed Changes

### MCP Client Core Integration

#### [NEW] [client.go](file:///Users/human/code/powerword/internal/mcp/client.go)
- Defines a wrapper struct `MCPClient` representing an active session with an MCP server.
- Integrates the SDK's initialization protocol (version handshake, declaration of client capabilities, and server initialization sequence).
- Implements capability fetching:
  - `ListTools(ctx context.Context)`
  - `CallTool(ctx context.Context, name string, arguments map[string]interface{})`

#### [NEW] [registry.go](file:///Users/human/code/powerword/internal/mcp/registry.go)
- Orchestrates multiple `MCPClient` connections.
- Serves as the single repository of active plugins, providing tools aggregated across all spawned MCP servers to the core execution loop.

---

## Verification Plan

### Automated Tests
- Mock MCP server handshakes to verify the client correctly performs standard version negotiation.
- Test aggregation logic in `registry.go` to ensure that name collision or empty tool list events are handled gracefully.

### Manual Verification
- Attempt to connect to a mock stdio-based test server, calling simple ping/echo tools.
