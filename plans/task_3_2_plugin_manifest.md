# plan: Task 3.2: Plugin Manifest Configuration & Registration

**Status:** Completed (Issue #52)


Establish a configuration-driven mechanism to discover, configure, and mount third-party or local MCP servers. Define a clean TOML configuration schema within `powerword.toml` to register these servers and their runtime parameters.

## User Review Required

> [!NOTE]
> Plugin configuration maps directly into the main config loading structure via Viper, using a designated `servers` configuration block in `powerword.toml`. The underlying Go logic is already implemented in Phase 1 & 2.

## Proposed Changes

### Configuration Mapping & Startup Logic

#### [MODIFY] [config.go](../internal/config/config.go)
- *Already Implemented*: Extends Config structures to support a map of Server definitions keyed by server name via the `servers` TOML block.
  ```toml
  [servers.filesystem]
     command = "pw-mcp-fs"
     args = ["--workspace", "/Users/human/code/powerword"]
     env = ["SOME_VAR=value"]

  [servers.memory]
     command = "npx"
     args = ["-y", "@modelcontextprotocol/server-memory"]
  ```

#### [MODIFY] [process.go](../internal/mcp/process.go) & [loop.go](../internal/loop/loop.go)
- *Already Implemented*: Parses the configuration definitions during boot in the `RunLoop`.
- *Already Implemented*: Locates executables in system `$PATH` or uses absolute paths to start server sub-processes.
- *Already Implemented*: Gracefully logs failures to initialize individual servers, keeping the rest of the working plugins operational.

---

## Verification Plan

### Automated Tests
- Test parsing various TOML syntax layouts (including environment variables, command lists, and custom arguments). Tests are available in `internal/config/config_test.go`.
- Ensure configuration loading handles empty server maps, and validates command paths.

### Manual Verification
- Create a `powerword.toml` file in the project root containing the following native plugins:
  ```toml
  [servers.filesystem]
     command = "go"
     args = ["run", "./cmd/pw-mcp-fs"]

  [servers.git]
     command = "go"
     args = ["run", "./cmd/pw-mcp-git"]

  [servers.shell]
     command = "go"
     args = ["run", "./cmd/pw-mcp-shell"]
  ```
- Run the CLI application to trigger the startup sequence:
  ```bash
  go run ./cmd/powerword
  ```
- To test the plugins interactively, run a query that requires them. For example:
  ```bash
  go run ./cmd/powerword "list the files in the current directory and read powerword.toml"
  ```
- Or pass the `--verbose` flag (or `POWERWORD_VERBOSE=true`) to see more detailed execution logging:
  ```bash
  go run ./cmd/powerword --verbose
  ```
- Verify the startup process successfully registers tools from these specified plugins without any `Warning: failed to start MCP server` errors.
