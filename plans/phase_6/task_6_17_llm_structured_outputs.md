# plan: Task 6.17: Shared LLM Structured Outputs & Schema Enforcement

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-12
**Unit Test Coverage:** 91.0%

Enhance the shared `pkg/llm` package in Powerword to support structured schema constraints. This enables clients (such as Pithos and Lamplighter) to request that LLMs strictly adhere to a specified Go struct or JSON schema format, utilizing provider-level JSON schema features (such as OpenAI Structured Outputs or Gemini Response Schema) to guarantee type-safety and eliminate unmarshaling errors.

Specifically, this schema enforcement layer will support complex client-defined struct formats such as:
1. Pithos's **Level 1 Art Style & Level 2 Character Consistency Profile** generator responses (e.g., matching a struct containing fields `style_seed` and `character_profile`).
2. Page-by-page parodic stanzas and illustration prompt list structures.

## User Review Required

> [!NOTE]
> This is a shared capability added to `pkg/llm` in Powerword. It does not introduce any breaking changes to existing raw text or basic JSON Mode APIs.

## Technical Details & Schema Reflection Blueprint

To support Go struct input to the JSON schema enforcer, the package will utilize `github.com/invopop/jsonschema` to reflect struct definitions dynamically.

### Helper Function: `generateJSONSchema`

Implement a helper in `pkg/llm/client.go`:
```go
import "github.com/invopop/jsonschema"

func generateJSONSchema(v any) (map[string]any, error) {
    reflector := jsonschema.Reflector{
        AllowAdditionalProperties:  false, // OpenAI and Gemini require additionalProperties: false
        RequiredFromJSONSchemaTags: true,
    }
    schema := reflector.Reflect(v)
    if schema == nil {
        return nil, fmt.Errorf("failed to reflect schema for type: %T", v)
    }

    // Marshal to JSON and unmarshal to map[string]any
    data, err := json.Marshal(schema)
    if err != nil {
        return nil, err
    }
    var result map[string]any
    if err := json.Unmarshal(data, &result); err != nil {
        return nil, err
    }
    return result, nil
}
```

### OpenAI Structured Outputs Integration

OpenAI requires `additionalProperties: false` recursively on all objects in the schema, and the root must define `strict: true`.

#### OpenAI Integration Details
- Refactor `Generate` to handle structured outputs:
  - If `cfg.ResponseSchema` is set:
    - Generate the schema map using `generateJSONSchema(cfg.ResponseSchema)`.
    - Configure the `openai.ChatCompletionRequest.ResponseFormat`:
      ```go
      req.ResponseFormat = &openai.ChatCompletionResponseFormat{
          Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
          JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
              Name:   "structured_output",
              Strict: true,
              Schema: schemaMap,
          },
      }
      ```

### Gemini Response Schema Integration

Gemini's Go SDK accepts a `*genai.Schema` structure for response schema constraints. We convert the reflected schema map into `*genai.Schema` using our existing `convertSchema` and `parseMapToSchema` helper routines.

#### Gemini Integration Details
- Refactor `Generate` to handle structured outputs:
  - If `cfg.ResponseSchema` is set:
    - Generate the schema map using `generateJSONSchema(cfg.ResponseSchema)`.
    - Call `convertSchema(schemaMap)` to translate the schema map into a `*genai.Schema`.
    - Apply it to the model before generation:
      ```go
      model.ResponseMIMEType = "application/json"
      model.ResponseSchema = convertedSchema
      ```

---

## Proposed Changes

### LLM Client Package

#### [MODIFY] [client.go](file://../../pkg/llm/client.go)
- Add `ResponseSchema` to `generateOptions`:
  ```go
  type generateOptions struct {
      ResponseMIMEType string
      ResponseSchema   any
  }
  ```
- Add a functional option `WithResponseSchema`:
  ```go
  // WithResponseSchema configures the model to strictly adhere to the provided schema definition.
  func WithResponseSchema(schema any) GenerateOption {
      return func(o *generateOptions) {
          o.ResponseSchema = schema
      }
  }
  ```
- Implement `generateJSONSchema` helper.

#### [MODIFY] [openai.go](file://../../pkg/llm/openai.go)
- Update `Generate` to compile and assign `ResponseFormat` with `JSONSchema` if `cfg.ResponseSchema` is configured.

#### [MODIFY] [gemini.go](file://../../pkg/llm/gemini.go)
- Update `Generate` to translate `ResponseSchema` via `convertSchema` and assign `ResponseSchema` on the underlying `genai.GenerativeModel`.

## Verification Plan

### Automated Tests
- Run tests in the `llm` package:
  ```bash
  go test -v ./pkg/llm/...
  ```
- Add unit tests in `openai_test.go` and `gemini_test.go` validating:
  * Passing functional options constructs the correct nested payload parameters.
  * Invalid schema structs (non-marshallable types) return proper compilation errors.
  * Correct formatting of schema fields (e.g. `additionalProperties: false`).
