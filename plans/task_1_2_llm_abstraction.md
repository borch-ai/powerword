# Task 1.2: LLM Abstraction Layer

Define a uniform abstraction interface to interact with multiple LLM providers (e.g. Gemini, OpenAI, Claude). Implement provider wrappers using their official Go clients or REST APIs.

## User Review Required

> [!IMPORTANT]
> The abstraction interface needs to support both single-turn generation, tool declarations/calls, and streaming content chunks.

## Proposed Changes

### LLM Interface & Provider Integrations

#### [NEW] [client.go](file:///Users/human/code/powerword/internal/llm/client.go)
- Defines structures:
  - `Message` (representing role, text content, and function/tool requests).
  - `ToolDefinition` (representation of tools formatted for LLM registration).
  - `ToolResponse` (result of a tool execution).
- Defines the `LLMClient` interface:
  ```go
  type LLMClient interface {
      Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error)
      Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan string, <-chan error, error)
  }
  ```

#### [NEW] [gemini.go](file:///Users/human/code/powerword/internal/llm/gemini.go)
- Implements `LLMClient` using the official Google GenAI Go SDK (`google.golang.org/api` or `github.com/google/generative-ai-go`).
- Handles conversation history and tool declarations mapped from/to standard structure.

#### [NEW] [openai.go](file:///Users/human/code/powerword/internal/llm/openai.go)
- Implements `LLMClient` using standard OpenAI Go SDK (`github.com/sashabaranov/go-openai`).
- Configures endpoints for OpenAI models, Anthropic models (via standard compatible API or custom wrappers), and local models (via Ollama/vLLM).

---

## Verification Plan

### Automated Tests
- Table-driven unit tests for payload formatting for both Gemini and OpenAI clients.
- Mock HTTP responses to ensure provider clients parse success and error conditions gracefully without real API credentials.

### Manual Verification
- Execute simple tests with active provider keys to verify message/response structures are correctly transmitted.
