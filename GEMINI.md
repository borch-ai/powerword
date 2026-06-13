# Gemini Agent Development Guide

Welcome, AI Agent! This document outlines the design patterns, architectural standards, and development guidelines for contributing to **Powerword**. Please read and adhere to these guidelines to ensure consistency, high quality, and readability of the codebase.

## Repository Overview

Powerword is a vendor-agnostic Agentic CLI written in Go. The directory layout follows standard Go project structure conventions:

```
powerword/
├── cmd/
│   └── powerword/          # Main CLI application entry point
│       └── main.go
├── internal/               # Private application and library code
│   ├── config/             # Cobra and Viper configuration management
│   ├── llm/                # LLM client abstraction and provider implementations
│   ├── loop/               # Core execution and reasoning loop
│   └── mcp/                # MCP client integration and process lifecycle management
├── pkg/                    # Public library code (if any, that can be used by external applications)
├── plans/                  # Detailed implementation plans for all roadmap tasks
│   └── ...
├── VISION.md               # High-level overview of the project vision and goals
├── ROADMAP.md              # Fleshed-out phase breakdown and milestones
└── GEMINI.md               # This file
```

---

## Architectural Principles

1. **Strict Decoupling via Interfaces:**
   - LLM providers (Gemini, Claude, OpenAI) must implement a common interface. Never couple the core execution loop to specific provider SDK types.
   - Define interfaces in the package where they are *consumed*, or in a minimal `internal` sub-package.
2. **Error Handling & Resilience:**
   - Always return errors rather than using panic. Panic is only allowed for unrecoverable setup errors during startup (e.g., config parsing failure if CLI cannot function).
   - Use `%w` to wrap errors to preserve root cause context.
   - Clean up resources (processes, network connections, temporary files) using `defer`.
3. **Explicit Configuration:**
   - Centralize config structures in [internal/config](file:///Users/human/code/powerword/internal/config).
   - Bind Viper to Cobra flags explicitly.
   - Use environment variables as overrides for all configurations (prefixed with `POWERWORD_`).
4. **Agent-Friendly Tooling Integration:**
   - Use the standard Model Context Protocol (MCP) Go SDK (`github.com/modelcontextprotocol/go-sdk`).
   - Treat external plugins as independent CLI processes communicating via standard I/O (stdio).
5. **Centralized Capability Architecture:**
   - Powerword is the sole repository for all shared AI capabilities (`pw-mcp-*` servers) inside the Borch-AI ecosystem. Sibling codebases (like Pithos and Kiln) must remain clean of low-level LLM client implementations or capability libraries. They consume these capability plugins as stdio subprocesses and import shared packages (such as `pkg/llm` and `pkg/telemetry`) directly, avoiding any duplication of core capability logic.

---

## Cross-Project Boundaries & Distinct Workflows

Powerword works in tandem with other applications (e.g., the Lamplighter dashboard). When contributing, strictly adhere to project boundaries:
- **Check Existing Plans First:** Before proposing or making any changes that might cross over into another repository, explicitly check the `plans/` directory and GitHub issues of *both* repositories. Do not duplicate work that is already planned or completed in the other project.
- **Isolate Changes:** Keep project codebases distinct. If a feature requires changes in multiple repositories (e.g., a frontend change in Lamplighter and a backend change in Powerword), create and maintain separate, isolated implementation plans in each respective repository. Do not merge their plans into a single file.

---

## Coding Guidelines

- **Go Version:** Go 1.26+
- **Format & Quality:** Ensure all Go code is formatted with `gofmt` and linted using `golangci-lint` (configured in [.golangci.yml](file:///Users/human/code/powerword/.golangci.yml)). The lint settings enforce `gosec` (AST security checks), `bodyclose` (prevent socket leaks), and `noctx` (ensure context propagation). Run with `make lint`.
- **Vulnerabilities:** Ensure code remains free of known package vulnerabilities by running the Go Vulnerability Scanner via `make vuln`.
- **Naming Conventions:**
  - Follow standard Go naming conventions (camelCase, acronyms capitalized like `LLM`, `CLI`, `MCP`).
  - Keep variable names short when scope is small (e.g., `ctx` for `context.Context`, `err` for `error`).
- **Context Propagation:**
  - Pass `context.Context` as the first parameter of any function making network calls, executing external processes, or participating in long-running execution loops.
- **Dependencies:**
  - Prefer Go standard library where possible.
  - Allowed third-party dependencies:
    - `github.com/spf13/cobra` (CLI)
    - `github.com/spf13/viper` (Config)
    - `github.com/modelcontextprotocol/go-sdk` (MCP)
    - `google.golang.org/api` (Gemini SDK)
    - `github.com/sashabaranov/go-openai` (OpenAI SDK)
    - `github.com/anthropics/anthropic-sdk-go` (Anthropic SDK)

---

## Testing Standards

- **Coverage Requirement:** We enforce a strict **91% unit test coverage** across the codebase.
- **Verification Commands:**
  - Run tests and check coverage threshold: `make check-coverage` (which utilizes the CLI subcommand `check-coverage`).
  - Generate visual HTML coverage report: `go tool cover -html=coverage.out` after running tests.
- **Unit Tests:**
  - Place unit tests in the same directory as the code under test (suffix `_test.go`).
  - Use tables for parameterized tests (Table-Driven Tests).
- **Mocking:**
  - Use interface mock generators or manually implement clean mock structs for LLM clients and MCP SDK interactions.
  - Avoid real network requests in unit tests. Use `httptest.NewServer` or mock clients.
- **Integration Tests:**
  - Place integrations tests under a separate test flow or use build tags (e.g., `//go:build integration`) to separate them from fast unit tests.

---

## Pull Request & Merging Workflow

- **PR Required for Mainline Changes:** Direct pushes to the remote `main` branch are blocked. AI agents and human contributors must **never** push changes directly to `main`. All updates, bug fixes, features, and documentation edits must go through a Pull Request.
- **Local Critic Review (Pre-Push):** Before pushing, the `powerword review --local` command runs automatically via a Git `pre-push` hook. It triggers a local build (`make all`), extracts the git diff using `pw-mcp-git`, and strictly evaluates the changes against the assigned implementation plan. Any failures or feedback are written to `.powerword-critic.md` (which is automatically cleaned up on success). Configure the critic in `config.toml` with `critic_provider`, `critic_model`, and `critic_endpoint`.
- **Never Bypass Hooks:** AI agents and human contributors must **never** use the `git push --no-verify` flag. The pre-push hook is a critical quality gate that must pass before pushing. If the hook fails, fix the underlying issues instead of bypassing it.
- **Pause Before Merging:** AI agents must **never** auto-merge pull requests. Always pause to request explicit user approval.
- **Review Copilot Feedback:** Review and address all comments, suggestions, or issues flagged by the GitHub Copilot Code Review runner before finalizing a task.
- **Review Loop with Timer:** After opening or updating a Pull Request, the agent should pause (e.g., schedule a 30-second timer) to check for Copilot Code Review comments. If no comments are found and the total wait time has not exceeded a 7-minute timeout, the agent should repeat the 30-second check loop. Once comments are found, or the timeout is reached with no comments, the agent applies necessary refactors, pushes the fixes, and restarts the check loop for the new commit.
- **Review Loop Iteration Requirement:** The review-and-fix process is iterative. The agent MUST repeat the polling-pause and comment-fetch loop for every commit pushed to the PR branch. The agent is forbidden from declaring a task complete or requesting user merge approval until a full polling loop has been completed on the latest commit with either zero new comments returned or all comments addressed.
- **Update Implementation Plan:** Before declaring a task complete or requesting merge approval, the agent/contributor must update the corresponding implementation plan in the [plans/](file:///Users/human/code/powerword/plans) directory to reflect the final choices, actual configurations, testing updates, and final Go version used.

---

## Quick Reference Links

- [VISION.md](file:///Users/human/code/powerword/VISION.md) - Project vision and core objectives.
- [ROADMAP.md](file:///Users/human/code/powerword/ROADMAP.md) - Detailed breakdown of phases and milestones.
- [Plans Directory](file:///Users/human/code/powerword/plans) - Individual implementation plans for roadmap tasks.
