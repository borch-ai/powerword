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

### Release Scripts & Automation

#### [NEW] [build_release_binaries.sh](file://../../scripts/build_release_binaries.sh)

- Introduce standalone executable bash script `scripts/build_release_binaries.sh` with strict error handling (`set -euo pipefail`).
- Dynamically tests package buildability via `CGO_ENABLED=0 go list "./$dir"` across `cmd/powerword` and `cmd/pw-mcp-*` without maintaining hardcoded exclusions.
- Cross-compiles each buildable package across target platforms (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`), injecting release version flags for `powerword` while plugins maintain their server protocol implementation versioning, and outputs to `dist/`.

#### [MODIFY] [release.yml](file://../../.github/workflows/release.yml)

- Invoke `make dist VERSION="$VERSION"` in the release workflow, keeping all CI build/lint steps uniform behind Makefile targets.
- The upload step `gh release upload` automatically picks up and publishes all files in `dist/*` as GitHub release assets.

#### [MODIFY] [Makefile](file://../../Makefile)

- Add `dist` target to invoke `./scripts/build_release_binaries.sh $(VERSION)`.
- Update `clean` target to clean `dist/`.

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
