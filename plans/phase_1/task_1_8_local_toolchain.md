# plan: Task 1.8: Local Toolchain Sandbox

**Status:** Completed (Issue #43)
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%

This plan details the setup of a pure, sandboxed local toolchain (`.tools/`) to download and manage the Node.js binaries exclusively for this repository.

## Motivation

Powerword frequently executes external MCP servers (often written in Node.js). Both DevContainers and Nix were rejected due to system environment constraints on macOS. To avoid global installations (`brew install node`), we will download the official Node binaries into a local `.tools` folder and temporarily add them to the `$PATH`.

## User Review Required

> [!NOTE]
> None for this task.

## Proposed Changes

### [NEW] [setup_toolchain.sh](file://../../scripts/setup_toolchain.sh)

- A short Bash script to fetch `node-v20.15.0-darwin-arm64.tar.gz`.
- Extracts it into `.tools/node`.
- Instructs the user to run `export PATH=$PWD/.tools/node/bin:$PATH` to use it.

## Verification Plan

1. Run `bash scripts/setup_toolchain.sh`.
2. Add `.tools/node/bin` to the PATH.
3. Validate presence of `node` and `npx`.
4. Ensure Powerword CLI successfully launches local MCP servers.
