# Powerword Roadmap

This roadmap defines the engineering journey to build **Powerword**, the vendor-agnostic Agentic CLI. It breaks down the system design into logical phases and actionable tasks, each accompanied by a detailed implementation plan.

---

## Phase 1: Foundation
Focus: Bootstrapping the CLI application, establishing the LLM interface layer, and building a basic prompt-response stream.

*   **Task 1.1: Project Initialization & Cobra/Viper Configuration**
    *   Set up the Go project structure, module, and command-line parsing structure with Cobra. Configure global settings (API keys, defaults) loaded via Viper.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_1_1_cobra_viper_setup.md)
*   **Task 1.2: LLM Abstraction Layer**
    *   Create a unified Go interface for interacting with LLM providers. Implement concrete providers for Gemini and OpenAI/Claude.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_1_2_llm_abstraction.md)
*   **Task 1.3: Core Loop & Streaming Output Engine**
    *   Implement the primary execution loop to pass user prompts to the abstraction layer, capture the stream, and render syntax-highlighted markdown back to the terminal.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_1_3_core_loop.md)

---

## Phase 2: Protocol Integration
Focus: Integrating the Model Context Protocol (MCP) and adapting the execution loop to orchestrate tools.

*   **Task 2.1: MCP Go SDK Integration**
    *   Integrate `modelcontextprotocol/go-sdk` as a library dependency. Structure the internal client code to discover and map server capabilities.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_2_1_mcp_integration.md)
*   **Task 2.2: Stdio Transport Layer & Server Lifecycle**
    *   Build standard I/O (stdio) transport handlers to launch, monitor, and clean up external MCP servers running as child processes.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_2_2_stdio_transport.md)
*   **Task 2.3: Tool Calling Execution Loop**
    *   Evolve the execution loop from standard stream to a multi-turn ReAct reasoning loop. Convert LLM tool calls to MCP requests, run the tools, and return execution results back to the LLM.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_2_3_tool_calling_loop.md)

---

## Phase 3: The Go Plugin Ecosystem
Focus: Delivering a standard set of native, high-performance Go MCP servers and a configuration schema for runtime discovery.

*   **Task 3.1: Standard Native Plugins (FS, Git, Shell)**
    *   Author a suite of lightweight Go-based MCP servers for reading local files, introspecting Git repositories, and executing shell commands with strict security profiles.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_3_1_go_plugins.md)
*   **Task 3.2: Plugin Manifest Configuration & Registration**
    *   Create a YAML-based plugins manifest schema. Enable the CLI to parse user configuration files to mount and spin up custom local MCP servers on launch.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_3_2_plugin_manifest.md)

---

## Phase 4: Advanced Agentic Features
Focus: Enhancing coordination, scaling capability, and enabling headless environments.

*   **Task 4.1: Multi-Model Orchestration & Intelligent Routing**
    *   Create a routing component to allocate tasks dynamically. For example, route simple context checks to smaller local/fast models, reserving large reasoning models for complex tool orchestration.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_4_1_multi_model.md)
*   **Task 4.2: Headless Pipelines & Automation**
    *   Support non-interactive pipeline execution modes. Allow Powerword to digest raw stdin stream arguments, output structured JSON format, and behave as a reliable utility in CI/CD environments.
    *   [Implementation Plan](file:///Users/human/code/powerword/plans/task_4_2_headless_pipeline.md)
