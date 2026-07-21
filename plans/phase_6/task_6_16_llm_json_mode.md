# plan: Task 6.16: LLM JSON Mode & Client Options

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.0%

Refactor `pkg/llm` in Powerword to support structured JSON generation via response MIME-type parameters, and ensure clients can be easily instantiated without tight coupling to Powerword's internal config.

## User Review Required

> [!NOTE]
> **Signature Breaking Change**:
> Modifying `LLMClient.Generate` signature breaks existing implementations. We will update all providers (Gemini, OpenAI, Anthropic) and all test mock implementations across the repository. Call sites will remain unaffected due to Go's variadic slice parameters (`opts ...GenerateOption`).

## Proposed Changes

### pkg/llm

#### [MODIFY] [client.go](file://../../pkg/llm/client.go)

- Add a `GenerateOption` functional options pattern to `Generate`:

  ```go
  type GenerateOption func(*generateOptions)

  type generateOptions struct {
      ResponseMIMEType string
  }

  func WithResponseMIMEType(mimeType string) GenerateOption {
      return func(o *generateOptions) {
          o.ResponseMIMEType = mimeType
      }
  }
  ```

- Update the `LLMClient` interface signature for `Generate` to accept variadic options:

  ```go
  Generate(ctx context.Context, messages []Message, tools []ToolDefinition, opts ...GenerateOption) (*Message, error)
  ```

#### [MODIFY] [gemini.go](file://../../pkg/llm/gemini.go)

- Update `Generate` to accept `opts ...GenerateOption`.
- In `prepareModel` (or inside `Generate`), apply the functional options:

  ```go
  cfg := &generateOptions{}
  for _, opt := range opts {
      opt(cfg)
  }
  if cfg.ResponseMIMEType != "" {
      model.GenerationConfig.ResponseMIMEType = cfg.ResponseMIMEType
  }
  ```

- Add support for custom base URLs in `GeminiClient` if needed by checking for custom endpoints.

#### [MODIFY] [openai.go](file://../../pkg/llm/openai.go)

- Update `Generate` to accept `opts ...GenerateOption`.
- In `prepareRequest`, parse the options. If `ResponseMIMEType` is set to `"application/json"`, set the response format parameter:

  ```go
  cfg := &generateOptions{}
  for _, opt := range opts {
      opt(cfg)
  }
  if cfg.ResponseMIMEType == "application/json" {
      req.ResponseFormat = &openai.ChatCompletionResponseFormat{
          Type: openai.ChatCompletionResponseFormatTypeJSONObject,
      }
  }
  ```

#### [MODIFY] [anthropic.go](file://../../pkg/llm/anthropic.go)

- Update `Generate` to accept `opts ...GenerateOption` (accept but ignore or handle as supported).

---

## Verification Plan

### Automated Tests

- Run `go test ./pkg/llm/...`
- Add a new unit test verifying that `Generate` correctly parses and applies `WithResponseMIMEType("application/json")` options to the underlying SDK request structures.
