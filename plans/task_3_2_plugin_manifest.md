# Task 3.2: Plugin Manifest Configuration & Registration

Establish a configuration-driven mechanism to discover, configure, and mount third-party or local MCP servers. Define a clean YAML configuration schema to register these servers and their runtime parameters.

## User Review Required

> [!NOTE]
> Plugin configuration will map directly into the main config loading structure via Viper, using a designated `mcp_servers` configuration block.

## Proposed Changes

### Configuration Mapping & Startup Logic

#### [MODIFY] [config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Extends Config structures to support an array of Server definitions:
  ```yaml
  mcp_servers:
    filesystem:
      command: "pw-mcp-fs"
      args: ["--workspace", "/Users/human/code/powerword"]
      env:
        SOME_VAR: "value"
    memory:
      command: "npx"
      args: ["-y", "@modelcontextprotocol/server-memory"]
  ```

#### [MODIFY] [registry.go](file:///Users/human/code/powerword/internal/mcp/registry.go)
- Parses the configuration definitions during boot.
- Locates executables in system `$PATH` or uses absolute paths to start server sub-processes.
- Gracefully logs failures to initialize individual servers, keeping the rest of the working plugins operational.

---

## Verification Plan

### Automated Tests
- Test parsing various YAML syntax layouts (including environment variables, command lists, and custom arguments).
- Ensure configuration loading handles empty server arrays, and validates command paths.

### Manual Verification
- Write a custom YAML config containing standard npm-based and native-based plugins. Launch Powerword and verify the startup logs show successfully registered tools from all specified plugins.
