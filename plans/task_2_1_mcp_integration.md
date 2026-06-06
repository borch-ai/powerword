# Task 2.1: MCP Go SDK Integration

Integrate the Model Context Protocol (MCP) Go SDK (`github.com/modelcontextprotocol/go-sdk`) into the Powerword binary. Establish client abstractions to initialize sessions, query capabilities, and translate schemas between LLM tools and MCP definitions.

## Status: Completed (Library Only - Not yet wired to CLI)

> [!NOTE]
> MCP client interactions are encapsulated under the [internal/mcp](../internal/mcp) package.
> We are using `github.com/modelcontextprotocol/go-sdk` version `v1.6.1` with `go 1.26.4`.

## Completed Changes

### MCP Client Core Integration

#### [NEW] [client.go](../internal/mcp/client.go)
- Defines a wrapper struct `MCPClient` representing an active session with an MCP server.
- Integrates the SDK's initialization protocol (version handshake, declaration of client capabilities, and server initialization sequence).
- Implements capability fetching:
  - `ListTools(ctx context.Context) ([]mcpsdk.Tool, error)`
  - `CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcpsdk.CallToolResult, error)`

#### [NEW] [registry.go](../internal/mcp/registry.go)
- Orchestrates multiple `MCPClient` connections.
- Serves as the single repository of active plugins, providing tools aggregated across all spawned MCP servers to the core execution loop.
- Aggregated tool names use the format `<client>__<tool>` to gracefully handle name collisions.

## Verification

### Automated Tests
- Implemented `client_test.go` and `registry_test.go` utilizing `mcp.NewInMemoryTransports()`.
- Test coverage for `internal/mcp` is **92.1%**, exceeding the 91% requirement.
- Mock MCP server handshakes verified standard version negotiation.
- Test aggregation logic in `registry.go` verified name collision resolution and routing logic.
