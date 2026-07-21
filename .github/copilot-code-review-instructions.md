# GitHub Copilot Code Review Instructions - Powerword

When reviewing pull requests in this repository, please enforce the following architectural principles, error handling patterns, and testing standards:

## 1. Strict Decoupling via Interfaces
* LLM provider implementations (Gemini, Claude, OpenAI) must implement the common provider interfaces.
* Never couple the core execution or reasoning loop to specific provider SDK client types.

## 2. Error Handling & Resilience
* Always return errors instead of using `panic`. Panic is only permitted for unrecoverable startup config failures.
* Wrap errors using `%w` to preserve original root cause context.
* Clean up resources (e.g., sockets, processes, temp files, file descriptors) using `defer` directly after creation.

## 3. Explicit Configuration & CLI Bindings
* Centralize configuration structures inside `internal/config` or `pkg/config`.
* Ensure new Cobra CLI flags are explicitly bound to Viper configurations.
* Enforce that environment variables overriding configurations are prefixed with `POWERWORD_`.

## 4. Coding & Testing Standards
* Go version target is 1.26+. All Go code must be formatted with `gofmt` and pass `golangci-lint` check rules.
* We enforce a strict **91% unit test coverage** threshold. Ensure new packages have corresponding `_test.go` files.
* Parameterized tests should be implemented using table-driven test configurations (`struct` inputs/expected outputs).
* Unit tests must never make live network calls or spawn live, un-mocked external subprocesses. Use `httptest.NewServer` or mock clients.
