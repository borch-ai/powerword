# plan: Task 6.5: WASM-Based Plugin Sandboxing

**Status:** Open (Issue #TBD)

This task integrates WebAssembly (WASM) execution transport for MCP plugins, allowing third-party Go plugins to run in an isolated WASM container instead of native shell subprocesses.

## User Review Required

> [!IMPORTANT]
> Running plugins under WASM requires mapping specific system hooks (like virtual directories) into the WASM runtime guest space. This requires adding a Go WASM runtime dependency (e.g. `github.com/tetratelabs/wazero`).

## Proposed Changes

### MCP Client and Transport

#### [NEW] [wasm.go](file://../../internal/mcp/wasm.go)

- [ ] Implement a WASM host transport wrapper parsing `.wasm` plugins using Wazero.
- [ ] Implement standard stdio message mapping from the WASM guest to the host MCP framework.

#### [MODIFY] [registry.go](file://../../internal/mcp/registry.go)

- [ ] Support loading `.wasm` plugins in addition to native executable configurations.

---

## Verification Plan

### Automated Tests

- [ ] Run `go test ./internal/mcp/...` compiling a small test WASM plugin and verifying stdio communication.

### Manual Verification

- [ ] Load a sample WASM plugin and check that tool requests are mapped correctly.
