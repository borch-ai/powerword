# Task 1.1: Project Initialization & Cobra/Viper Configuration

Establish the Go module foundation, set up the standard directory structure, and implement command-line routing and configuration management using Cobra and Viper.

## User Review Required

> [!IMPORTANT]
> Configuration Location: By default, configuration will load from `~/.config/powerword/config.yaml`. We should support dynamic path loading using a `--config` global flag.

## Proposed Changes

### Go Module & CLI Scaffolding

#### [NEW] [go.mod](file:///Users/human/code/powerword/go.mod)
- Standard Go 1.22 module declaration.
- Add dependencies for Cobra and Viper.

#### [NEW] [main.go](file:///Users/human/code/powerword/cmd/powerword/main.go)
- Entry point of the CLI application.
- Invokes the Cobra execute command.

#### [NEW] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Defines the root command using `spf13/cobra`.
- Handles flags (e.g., `--config`, `--model`, `--verbose`).
- Calls Viper config initializer.

#### [NEW] [config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Holds config structures (e.g., `Config` struct mapping API keys, default models, and plugin setups).
- Logic to bind environment variables (`POWERWORD_`) to config parameters.
- Validates the environment requirements (checks if at least one API key is present).

---

## Verification Plan

### Automated Tests
- `go test ./internal/config/...` - Validate that environment variables override configuration files.
- `go test ./cmd/...` - Mock configuration reading and ensure the app starts up without errors.

### Manual Verification
- Compile and run `$ ./powerword --help` to confirm Cobra flag output.
- Create a test `config.yaml` file, load it via `--config`, and verify loaded values via verbose logging.
