# plan: Task 6.14: Publish Automated Binary Releases of MCP Plugins

**Status:** Open

Extend the GitHub Actions release workflow in Powerword to compile, package, and publish the suite of native Go MCP servers (`pw-mcp-fs`, `pw-mcp-git`, `pw-mcp-shell`, `pw-mcp-imagegen`, `pw-mcp-kdp-math`, `pw-mcp-seo`, `pw-mcp-viral`, `pw-mcp-critic`, `pw-mcp-epub`, `pw-mcp-pdfcheck`) along with the core `powerword` binary.

## User Review Required

> [!NOTE]
> None. This purely adds compilation and artifact upload routines to the existing release pipeline.

## Proposed Changes

### GitHub Actions Workflow

#### [MODIFY] [release.yml](file://../../.github/workflows/release.yml)

- Update the build step to loop over all plugins under `cmd/` and cross-compile them.
- Output compiled binaries to the `dist` folder:
  - `${plugin}-darwin-amd64`
  - `${plugin}-darwin-arm64`
  - `${plugin}-linux-amd64`
  - `${plugin}-linux-arm64`
  - `${plugin}-windows-amd64.exe`
- The upload step `gh release upload` will automatically pick up and publish these new files.

---

## Verification Plan

### Automated Tests

- Validate that standard project compilation works by executing:

  ```bash
  make build
  ```

### Manual Verification

- Check CI output of a release push to verify all binaries are built successfully and attached as release assets.
