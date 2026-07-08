# plan: Task 3.24: Local Semantic Memory Plugin (`pw-mcp-memory`)

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-07-07
**Unit Test Coverage:** 91.10%

## Goal Description

Build a native Go Model Context Protocol (MCP) server `pw-mcp-memory` exposing semantic search tools. It will allow the Powerword CLI to store, search, and recall past conversational context, notes, or documentation using a lightweight, local in-process vector space model or embedded database.

## User Review Required

> [!IMPORTANT]
> **Vector Database Dependency**:
> We propose using a pure Go implementation of cosine-similarity matching on local arrays (or a lightweight embedded DB like SQLite with standard blob storage) to avoid introducing heavy external CGO-dependent vector DB libraries.
>
> **Embedding Sourcing**:
> The plugin requires access to an embedding model (e.g. OpenAI's `text-embedding-3-small` or Gemini's `text-embedding-004`). The API keys are sourced from standard environment variables matching LLM configurations.

---

## Proposed Changes

### Sibling: Powerword (`pkg/llm`)

#### [MODIFY] [client.go](file://../../pkg/llm/client.go)
* Add `Embed(ctx context.Context, texts []string) ([][]float32, error)` to `LLMClient` interface.
#### [MODIFY] [gemini.go](file://../../pkg/llm/gemini.go)
* Implement `Embed` using `text-embedding-004`.
#### [MODIFY] [openai.go](file://../../pkg/llm/openai.go)
* Implement `Embed` using `text-embedding-3-small`.
#### [MODIFY] [anthropic.go](file://../../pkg/llm/anthropic.go)
* Implement `Embed` (returns error as Anthropic does not support generic embeddings here).


### Sibling: Powerword (`cmd/pw-mcp-memory`)

#### [NEW] [main.go](file://../../cmd/pw-mcp-memory/main.go)
* Initializes the MCP server using the Go SDK.
* Implements tools:
  * `memory_add`:
    * Arguments: `text`, `tags` (optional slice of strings).
    * Implementation: Calls LLM embedding endpoint, computes vector, and persists the payload (text, tags, vector, timestamp) to a local JSON file in `~/.local/share/powerword/memory.json`.
  * `memory_search`:
    * Arguments: `query`, `limit` (int), `min_similarity` (float).
    * Implementation: Generates embedding for query, computes cosine similarity against all stored vector embeddings, and returns the top matches sorted by similarity score.

#### [NEW] [main_integration_test.go](file://../../cmd/pw-mcp-memory/main_integration_test.go)
* Runs an integration test using the MCP StdioTransport to ensure tools are properly exposed.


---

## Verification Plan

### Automated Tests
* Create unit tests in `cmd/pw-mcp-memory/main_test.go`:
  * Mock the LLM embedding provider client.
  * Verify that `memory_add` writes records to local storage.
  * Verify that `memory_search` computes correct cosine similarity scores and returns sorted results.

### Manual Verification
1. Build the binary:
   ```bash
   go build -o bin/pw-mcp-memory cmd/pw-mcp-memory/main.go
   ```
2. Test calling `memory_add` with text and query it via `memory_search` to verify top matches are returned.
