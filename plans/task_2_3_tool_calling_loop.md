# Task 2.3: Tool Calling Execution Loop

Evolve the single-turn core loop into an iterative reasoning loop (ReAct loop). Integrate the plugin registry, fetch available tools, declare them to the active LLM, execute requested tool actions via MCP servers, and feed the results back to the LLM until generation is complete.

## User Review Required

> [!CAUTION]
> Safety Guardrails: Executing commands, write operations, and file removals must have strict confirmation steps or guardrails. The loop configuration must specify default policies (e.g., auto-confirm or manual prompt for destructive actions).

## Proposed Changes

### ReAct Reasoning Loop

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Extends the `RunLoop` execution logic.
- Loops sequentially up to a configurable max limit (e.g. 10 turns) to prevent infinite tooling loops.
- Manages LLM context window by accumulating both assistant prompt turns, tool invocations, and tool results.
- Prompts the user before executing tools marked as "unsafe" (if custom configuration mandates user approval).

#### [NEW] [translator.go](file:///Users/human/code/powerword/internal/mcp/translator.go)
- Converts provider-specific function-call arguments (e.g. Gemini tool calls, OpenAI tool calls) into standard JSON schemas required by MCP servers.
- Translates the MCP tool execution response back into the format expected by the respective LLM API.

---

## Verification Plan

### Automated Tests
- Mock model client responses representing tool-call sequences to ensure the agent executes exactly the expected tool, appends results, and issues the final response.
- Verify infinite loop protection is triggered if the model continues to request tool invocations past the maximum permitted limit.

### Manual Verification
- Mount a dummy tool (e.g., `get_current_weather`), ask the model: `"What is the weather in Paris, and should I carry an umbrella?"`, and watch the model trigger the tool call, receive output, and synthesize the final answer.
