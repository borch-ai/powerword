# Task 1.7: Models Subcommand

**Status:** Completed (Issue #42)

## Objective
Implement a `powerword models` subcommand to act as a pre-flight check for users to verify their configured API key can access the intended models. 

## User Review Required
No specific user review is required for this minor feature addition, other than standard PR review.

## Proposed Changes
### cmd/powerword
- Added `cmd/powerword/models.go` containing `newModelsCmd()`.
- Wires the command to the root command in `cmd/powerword/main.go`.

### internal/llm
- Extended `internal/llm/client.go` to include `ListModels(ctx context.Context) ([]string, error)`.
- Implemented `ListModels` in `Anthropic Provider`, `OpenAI Provider`, and `Gemini Provider`.

### internal/config
- Updated `PersistentPreRunE` in `internal/config/root.go` to properly load configurations for subcommands instead of skipping config parsing when there are no arguments.

## Verification Plan
### Automated Tests
- Added `TestAnthropicClient_ListModels_Success` and `TestAnthropicClient_ListModels_Error` to `anthropic_test.go`.
- Added `TestOpenAIClient_ListModels_Success` and `TestOpenAIClient_ListModels_Error` to `openai_test.go`.
- Added `TestGeminiClient_ListModels_Success` and `TestGeminiClient_ListModels_Error` to `gemini_test.go`.
- Extended `mockLLMClient` in `internal/loop/loop_test.go` to satisfy the updated `LLMClient` interface.
- Added `TestRootCmd_SubcommandConfigLoading` in `internal/config/root_test.go` to ensure correct loading of configuration when executing subcommands.
- Verified test coverage is > 91% (`91.70%`).

### Manual Verification
- Execute `powerword models` manually to verify it successfully lists the models for the configured provider.

## Environment
- **Go Version:** 1.26.4
