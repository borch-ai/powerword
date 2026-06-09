# Task 1.2: LLM Abstraction Layer

**Status:** Completed (Issue #37)

Define a uniform abstraction interface to interact with multiple LLM providers (Gemini, OpenAI, and Anthropic). Implement provider wrappers using their Go SDKs (the official SDKs for Gemini and Anthropic, and the popular community SDK for OpenAI).

## User Review Required

No review required as this task is completed.

## Actual Choices & Configurations

- **Go Version Used**: Go 1.26.4
- **Third-Party SDK Dependencies Added**:
  - `github.com/google/generative-ai-go/genai` (v0.20.1) for Gemini
  - `github.com/sashabaranov/go-openai` (v1.41.2) for OpenAI
  - `github.com/anthropics/anthropic-sdk-go` (v1.46.0) for Anthropic (Claude)
  - `google.golang.org/api/option` (v0.283.0) for initializing Google client options

- **The Abstraction Interface**:
  - Defined in `internal/llm/client.go` with uniform structures `Role`, `Message`, `ToolCall`, `ToolDefinition`, and `StreamChunk`.
  - `LLMClient` Interface definition:
    ```go
    type LLMClient interface {
        Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error)
        Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error)
    }
    ```
  - Factory function `NewClient(cfg *config.Config) (LLMClient, error)` automatically resolves the target provider based on the configured model name:
    - Model names containing `"gemini"` route to the Gemini client wrapper.
    - Model names containing `"claude"` route to the Anthropic client wrapper.
    - Other models route to the OpenAI client wrapper.

## Proposed Changes

### LLM Interface & Provider Integrations

#### [MODIFY] [client.go](file:///Users/human/code/powerword/internal/llm/client.go)
- Defines structures:
  - `Role` (`system`, `user`, `assistant`, `tool`).
  - `Message` representing role, text content, tool calls, and tool response associations.
  - `ToolDefinition` representing tool metadata.
  - `ToolCall` for function call payloads.
  - `StreamChunk` holding streamed content or stream errors.
- Defines the `LLMClient` interface.
- Implements `NewClient` resolver logic.

#### [NEW] [gemini.go](file:///Users/human/code/powerword/internal/llm/gemini.go)
- Implements `LLMClient` using the official Google GenAI Go SDK.
- Recursively converts standard JSON Schemas to `*genai.Schema`.
- Maps conversation history, handles tool calls/responses, and streams responses via `SendMessageStream`.

#### [NEW] [openai.go](file:///Users/human/code/powerword/internal/llm/openai.go)
- Implements `LLMClient` using `github.com/sashabaranov/go-openai`.
- Maps messages, tool definitions, tool calls, and streams chat completions.

#### [NEW] [anthropic.go](file:///Users/human/code/powerword/internal/llm/anthropic.go)
- Implements `LLMClient` using the official `github.com/anthropics/anthropic-sdk-go` client.
- Maps messages, system prompts, tool schemas, and handles streaming/message deltas.

---

## Verification Plan

### Automated Tests
We implemented extensive table-driven unit tests with simulated HTTP mock servers using `httptest.NewServer` to mock API responses and verify correct request formats/streaming:
- [client_test.go](file:///Users/human/code/powerword/internal/llm/client_test.go): Tests the client resolver factory.
- [gemini_test.go](file:///Users/human/code/powerword/internal/llm/gemini_test.go): Mocks Gemini REST responses for generate content, function calling, schema parsing, and event stream.
- [openai_test.go](file:///Users/human/code/powerword/internal/llm/openai_test.go): Mocks OpenAI completions, streaming, and tool calls.
- [anthropic_test.go](file:///Users/human/code/powerword/internal/llm/anthropic_test.go): Mocks Anthropic SSE streaming, messages creation, and tool results.

- **Verification Command**: Run `make check-coverage`
- **Result**: Statement coverage is **91.4%**, meeting the strict coverage threshold requirement of **91.0%**.

### Manual Verification
- Verified code compilation of all packages by running `go build ./...` which passes with no compile errors.
