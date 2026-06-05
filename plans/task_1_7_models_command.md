# Task 1.7: Models Subcommand

## Objective
Implement a `powerword models` subcommand to act as a pre-flight check for users to verify their configured API key can access the intended models. 

## Final Choices & Implementation Details
- **LLMClient Interface**: Extended `internal/llm/client.go` to include `ListModels(ctx context.Context) ([]string, error)`.
- **Anthropic Provider**: Implemented `ListModels` using `client.Models.List`.
- **OpenAI Provider**: Implemented `ListModels` using `client.ListModels`.
- **Gemini Provider**: Implemented `ListModels` using `client.ListModels(ctx)` and iterating through the results.
- **CLI Subcommand**: Added `cmd/powerword/models.go` containing `newModelsCmd()`.
- **Wiring**: Added the command to the root command in `cmd/powerword/main.go`. Updated `PersistentPreRunE` in `internal/config/root.go` to properly load configurations for subcommands instead of skipping config parsing when there are no arguments.

## Testing Updates
- Added `TestAnthropicClient_ListModels_Success` and `TestAnthropicClient_ListModels_Error` to `anthropic_test.go`.
- Added `TestOpenAIClient_ListModels_Success` and `TestOpenAIClient_ListModels_Error` to `openai_test.go`.
- Added `TestGeminiClient_ListModels_Success` and `TestGeminiClient_ListModels_Error` to `gemini_test.go`.
- Extended `mockLLMClient` in `internal/loop/loop_test.go` to satisfy the updated `LLMClient` interface.
- Added `TestRootCmd_SubcommandConfigLoading` in `internal/config/root_test.go` to ensure correct loading of configuration when executing subcommands.
- Verified test coverage is > 91% (`91.70%`).

## Environment
- **Go Version:** 1.26.4
