# plan: Task 2.3: Tool Calling Execution Loop

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

**1. Setup Environment**
- If you don't have Node installed globally, add the local toolchain to your path: `export PATH=$PWD/.tools/node/bin:$PATH` (run `bash scripts/setup_toolchain.sh` first if the folder is missing).
- Create a configuration `powerword.toml` that mounts an MCP server, such as the `@modelcontextprotocol/server-everything` npx package. 
- Enable verbose logging using `POWERWORD_VERBOSE=true` to observe background tool invocations.

**2. Basic Tool Execution**
- Run `powerword "Echo 'hello world' using the echo tool"` (using the `echo` tool from the everything server).
- Verify that the verbose logs show the tool being called with the correct arguments.
- Verify that the final response contains the expected output synthesized by the LLM.

**3. Tool Error Handling**
- Ask the model to execute an action that will intentionally fail (e.g., trying to read a file that doesn't exist using the filesystem MCP server, or passing bad arguments).
- Verify that the error returned by the MCP server is caught, sent back to the LLM, and the LLM gracefully handles the failure rather than crashing the loop.

**4. Loop Limit Protection**
- Temporarily lower the max ReAct loop iteration limit to `2` in the code or configuration.
- Prompt the model with a complex task that requires multiple steps, such as exploring a directory tree structure.
- Verify that the CLI stops execution when the loop limit is reached, emitting a clear warning or error about exceeding maximum loop iterations.

**5. Parallel / Sequential Tool Calling**
- Run a prompt requiring multiple disjoint facts: `"Fetch the weather for New York, Paris, and Tokyo."` (using a dummy weather tool).
- Verify in the verbose logs that the tool calls are dispatched (either sequentially or in parallel depending on the API provider's capabilities), and that their aggregated results are passed back to the model correctly.

---

## Completion Status

Task 2.3 is **COMPLETED** (Issue #46).

### Final Design Decisions
1. **Streaming vs Generate**: The `RunLoop` was updated to utilize blocking `Generate` requests instead of incremental `Stream` during the loop. This guarantees tools are reliably passed in and handled iteratively without requiring a complex parser for partial tool-call streams.
2. **Loop Iterations Limits**: Added `MaxLoopIterations` (default: 10) to `config.Config` to protect against infinite loops.
3. **Safety AutoConfirm**: Added `AutoConfirm` (default: `false`) to pause and prompt the user in terminal (`y/N`) before actually dispatching MCP tools.
4. **Translation Layer**: Created `internal/mcp/translator.go` to adapt `mcpsdk.Tool` into unified generic `llm.ToolDefinition`s.
