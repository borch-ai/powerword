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

---

## Coding Guidelines

- **Go Version:** Go 1.23+
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

---

## Testing Standards

- **Coverage Requirement:** We enforce a strict **91% unit test coverage** across the codebase.
- **Verification Commands:**
  - Run tests and check coverage threshold: `make check-coverage` (which utilizes the custom validator script [check_coverage.go](file:///Users/human/code/powerword/scripts/check_coverage.go)).
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

## Quick Reference Links

- [VISION.md](file:///Users/human/code/powerword/VISION.md) - Project vision and core objectives.
- [ROADMAP.md](file:///Users/human/code/powerword/ROADMAP.md) - Detailed breakdown of phases and milestones.
- [Plans Directory](file:///Users/human/code/powerword/plans) - Individual implementation plans for roadmap tasks.
