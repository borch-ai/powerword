# plan: Task 1.1: Project Initialization & Cobra/Viper Configuration (TOML)

**Status:** Completed (Issue #36)
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%

Establish the Go module foundation, set up the standard directory structure, and implement command-line routing and configuration management using Cobra and Viper with TOML formatting.

## User Review Required

> [!IMPORTANT]
> - **Configuration Format**: Following user feedback, the configuration format is switched from YAML to TOML.
> - **Default Path**: The default configuration path is set to `~/.config/powerword/config.toml`.
> - **Environment Bindings**: We explicitly map env variables starting with `POWERWORD_` to override the TOML/flag settings.
> - **.env File Support**: The CLI dynamically parses `.env` files in the current working directory at startup, only loading keys starting with `POWERWORD_` and never overwriting existing environment variables.

## Proposed Changes

### Go Module & CLI Scaffolding

#### [NEW] [go.mod](file:///Users/human/code/powerword/go.mod)
- Standard Go 1.26.4 module declaration.
- Add dependencies for Cobra and Viper.

#### [NEW] [main.go](file:///Users/human/code/powerword/scripts/lint_markdown/main.go)
- Entry point of the CLI application.
- Invokes the Cobra execute command.

#### [NEW] [root.go](file:///Users/human/code/powerword/pkg/config/root.go)
- Defines command structure via a fresh command constructor `NewRootCmd()` to avoid static global state issues in tests.
- Handles flags (e.g., `--config`, `--model`, `--verbose`).
- Skips configuration setup on empty arguments to print help directly.
- Directs outputs to `cmd.Printf` instead of `fmt.Printf`.

#### [NEW] [config.go](file:///Users/human/code/powerword/pkg/config/config.go)
- Holds config structures (`Config` struct mapping API keys, default models, and plugin setups).
- Dynamically loads `.env` files using Viper's properties parser.
- Explicitly handles `BindEnv` errors.
- Validates the environment requirements (checks if at least one API key is present).

---

## Verification Plan

### Automated Tests
- `go test ./internal/config/...` - Validate config loading, dotenv parsing, and environment variable overrides.
- `make check-coverage` - Verify that test statement coverage meets or exceeds 91%.
- `make lint` - Validate golangci-lint compliance.
- `make vuln` - Check for security vulnerabilities.

### Manual Verification
- Compile the binary: `make build`
- Run binary help: `./bin/powerword --help`
- Run with dynamic configuration to test file parsing:
  - Create a temporary `.env` file under `/Users/human/code/powerword/.env` with mock keys.
  - Execute `./bin/powerword --verbose` and verify loaded values.
