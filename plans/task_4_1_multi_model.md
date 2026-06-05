# Task 4.1: Multi-Model Orchestration & Intelligent Routing

Develop routing mechanisms to dispatch prompts to different models dynamically. For example, route lightweight validation or tool parameter analysis to fast local models (like Ollama/vLLM), and complex, multi-turn tool reasoning steps to a frontier model (like Gemini 1.5 Pro).

## User Review Required

> [!IMPORTANT]
> Designing the routing strategy requires careful classification of task complexity. We will allow users to define routing rules via regex patterns or semantic categorization.

## Proposed Changes

### Model Router

#### [NEW] [router.go](file:///Users/human/code/powerword/internal/llm/router.go)
- Implements `ModelRouter` containing routing policy rules (e.g. mapping tasks like code linting, git logs, or database lookups to specific models).
- Implements classification logic:
  - Direct prompt-based classification (asking a very fast local model to categorize task complexity).
  - Rule-based routing based on CLI flags or explicit user prefixes.

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Integrates `ModelRouter` inside the execution loop.
- Supports hot-swapping the active LLM client mid-session when the nature of the conversation shifts from planning/exploring to execution.

---

## Verification Plan

### Automated Tests
- Test parsing and applying routing rules against static user prompt logs.
- Verify that state (conversation history) is correctly serialized/deserialized when passing context between two different model client types (e.g., passing history from an OpenAI-style model to a Gemini-style model).

### Manual Verification
- Execute a query with explicit routing: `$ powerword --route="git=local,code=gemini" "List files changed in git and write a docstring for each"`. Confirm terminal debug logs show correct routing endpoints being called for each segment.
