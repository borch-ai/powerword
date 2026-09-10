# plan: Task 6.14: Publish Automated Binary Releases of MCP Plugins

**Status:** Completed
**Go Version:** 1.26.6
**Date Completed:** 2026-09-10
**Unit Test Coverage:** 91.00%

Extend the GitHub Actions release workflow in Powerword to compile, package, and publish the suite of native Go MCP servers (`pw-mcp-*`) along with the core `powerword` binary.

## User Review Required

> [!NOTE]
> None. This purely adds compilation and artifact upload routines to the existing release pipeline.

## Proposed Changes

### GitHub Actions Workflow

#### [MODIFY] [release.yml](file://../../.github/workflows/release.yml)

- Update the build step to loop over all plugins under `cmd/` and cross-compile them across target architectures (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`), dynamically testing buildability via `go list` under `CGO_ENABLED=0` to include all pure-Go commands without maintaining hardcoded exclusions.
- Output compiled binaries to the `dist` folder:
  - `${plugin}-darwin-amd64`
  - `${plugin}-darwin-arm64`
  - `${plugin}-linux-amd64`
  - `${plugin}-linux-arm64`
  - `${plugin}-windows-amd64.exe`
- The upload step `gh release upload` automatically picks up and publishes these files as GitHub release assets.

---

## Verification Plan

### Automated Tests

- Validate that standard project compilation works by executing:

  ```bash
  make build
  make check-coverage
  make markdown-lint
  make lint
  ```

### Manual Verification

- Verify that CI build scripts correctly find all pure-Go commands and that the release pipeline YAML passes linting and formatting standards.
