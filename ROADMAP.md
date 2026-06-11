# Powerword Roadmap

This roadmap defines the engineering journey to build **Powerword**, the vendor-agnostic Agentic CLI. It breaks down the system design into logical phases and actionable tasks, each accompanied by a detailed implementation plan.

---

## Phase 1: Foundation
Focus: Bootstrapping the CLI application, establishing the LLM interface layer, and building a basic prompt-response stream.

*   **Task 1.1: Project Initialization & Cobra/Viper Configuration**
    *   Set up the Go project structure, module, and command-line parsing structure with Cobra. Configure global settings (API keys, defaults) loaded via Viper.
    *   [Implementation Plan](plans/task_1_1_cobra_viper_setup.md)
*   **Task 1.2: LLM Abstraction Layer**
    *   Create a unified Go interface for interacting with LLM providers. Implement concrete providers for Gemini and OpenAI/Claude.
    *   [Implementation Plan](plans/task_1_2_llm_abstraction.md)
*   **Task 1.3: Core Loop & Streaming Output Engine**
    *   Implement the primary execution loop to pass user prompts to the abstraction layer, capture the stream, and render syntax-highlighted markdown back to the terminal.
    *   [Implementation Plan](plans/task_1_3_core_loop.md)
*   **Task 1.4: Session Persistence & Chat History Management**
    *   Implement a local persistence engine (JSON files or SQLite database) in `~/.local/share/powerword/sessions` to store chat history, session state, and model parameters. Support resuming past sessions using a `--session` CLI flag.
    *   [Implementation Plan](plans/task_1_4_session_persistence.md)
*   **Task 1.5: Markdown Linting & Implementation Plan Validation**
    *   Integrate Markdown linting tooling (`markdownlint-cli`) into the repository gates. Establish an automated structure check script to verify that all implementation plans conform to the core template standard.
    *   [Implementation Plan](plans/task_1_5_markdown_linting.md)
*   **Task 1.8: Local Toolchain Sandbox**
    *   Create a local, sandboxed script to fetch and run Node.js locally without any system-wide packages or containers.
    *   [Implementation Plan](plans/task_1_8_local_toolchain.md)

---

## Phase 2: Protocol Integration
Focus: Integrating the Model Context Protocol (MCP) and adapting the execution loop to orchestrate tools.

*   **Task 2.1: MCP Go SDK Integration**
    *   Integrate `modelcontextprotocol/go-sdk` as a library dependency. Structure the internal client code to discover and map server capabilities.
    *   [Implementation Plan](plans/task_2_1_mcp_integration.md)
*   **Task 2.2: Stdio Transport Layer & Server Lifecycle**
    *   Build standard I/O (stdio) transport handlers to launch, monitor, and clean up external MCP servers running as child processes.
    *   [Implementation Plan](plans/task_2_2_stdio_transport.md)
*   **Task 2.3: Tool Calling Execution Loop**
    *   Evolve the execution loop from standard stream to a multi-turn ReAct reasoning loop. Convert LLM tool calls to MCP requests, run the tools, and return execution results back to the LLM.
    *   [Implementation Plan](plans/task_2_3_tool_calling_loop.md)
*   **Task 2.4: Interactive Permission & Consent Manager**
    *   Build a CLI permission prompt engine to intercept tool executions (like filesystem edits, commands, or network hits). Prompt the user interactively in the terminal before running unsafe tools.
    *   [Implementation Plan](plans/task_2_4_permission_manager.md)

---

## Phase 3: The Go Plugin Ecosystem & Systems Integration
Focus: Delivering a standard set of native, high-performance Go MCP servers, a configuration schema, and specialized operations tools.

*   **Task 3.1: Standard Native Plugins (FS, Git, Shell)**
    *   Author a suite of lightweight Go-based MCP servers for reading local files, introspecting Git repositories, and executing shell commands with strict security profiles.
    *   [Implementation Plan](plans/task_3_1_go_plugins.md)
*   **Task 3.2: Plugin Manifest Configuration & Registration**
    *   Create a YAML-based plugins manifest schema. Enable the CLI to parse user configuration files to mount and spin up custom local MCP servers on launch.
    *   [Implementation Plan](plans/task_3_2_plugin_manifest.md)
*   **Task 3.3: Kubernetes Diagnostician Plugin (K8s)**
    *   Implement a native Go MCP server to communicate with local/remote Kubernetes clusters to inspect namespaces, check service states, dump failing pod logs, and run diagnostics.
    *   [Implementation Plan](plans/task_3_3_k8s_mcp.md)
*   **Task 3.4: SQL Database Inspector Plugin**
    *   Build a database bridge Go MCP server supporting Postgres, MySQL, and SQLite. Support listing schemas, describing table structures, executing read-only check queries, and alerting on locks or long-running queries.
    *   [Implementation Plan](plans/task_3_4_db_mcp.md)
*   **Task 3.5: Creative Asset Generation Plugin (ImageGen)**
    *   Implement a native Go MCP server supporting OpenAI's DALL-E 3 and custom Midjourney wrappers to generate, catalog, and query consistent style referenced images.
    *   [Implementation Plan](plans/task_3_5_imagegen_mcp.md)
*   **Task 3.6: KDP Book Geometry & PDF Validator Plugin (KDP Math)**
    *   Implement a native Go MCP server to calculate exact cover, interior, margins, and bleed specs for Amazon KDP print books, and run validation on final compiled PDF page dimensions.
    *   [Implementation Plan](plans/task_3_6_kdp_math_mcp.md)
*   **Task 3.7: Viral Promo Asset Builder Plugin (Viral)**
    *   Implement a native Go MCP server wrapping TTS engines (ElevenLabs) and generative video tools to synthesize ASMR narration and stitch promotional trailers using local ffmpeg commands.
    *   [Implementation Plan](plans/task_3_7_viral_mcp.md)
*   **Task 3.8: Amazon KDP SEO & Metadata Agent Plugin (SEO)**
    *   Implement a native Go MCP server querying keyword volumes and product search suggestions to formulate listing titles, descriptions, and tag payloads.
    *   [Implementation Plan](plans/task_3_8_seo_mcp.md)

---

## Phase 4: Operations Automation & Advanced Features
Focus: Enhancing coordinator routing, telemetry, cloud integration, and non-interactive execution engines.

*   **Task 4.1: Multi-Model Orchestration & Intelligent Routing**
    *   Create a routing component to allocate tasks dynamically. For example, route simple context checks to smaller local/fast models, reserving large reasoning models for complex tool orchestration.
    *   [Implementation Plan](plans/task_4_1_multi_model.md)
*   **Task 4.2: Headless Pipelines & Automation**
    *   Support non-interactive pipeline execution modes. Allow Powerword to digest raw stdin stream arguments, output structured JSON format, and behave as a reliable utility in CI/CD environments.
    *   [Implementation Plan](plans/task_4_2_headless_pipeline.md)
*   **Task 4.3: Telemetry, Token Metrics & Cost Accounting**
    *   Implement usage accounting to track input, output, and cached tokens consumed during loops. Calculate and display cost metrics on execution exit.
    *   [Implementation Plan](plans/task_4_3_telemetry_cost.md)
*   **Task 4.4: Cloud Orchestrator Plugin (AWS/GCP)**
    *   Create a lightweight Go MCP server to parse cloud console resource metadata (EC2/GCE states, cloud watch logs, storage buckets) to query deployment status.
    *   [Implementation Plan](plans/task_4_4_cloud_mcp.md)

---

## Phase 5: Autonomous Review & Repair Loop
Focus: Delivering a fully autonomous local-to-remote review feedback and code correction pipeline driven by GitHub Issues.

*   **Task 5.1: Structured Issue Templates & Local Critic Integration**
    *   Define GitHub YAML Issue Forms to enforce structured planning schemas, and implement a `powerword review` sub-command to parse active issue fields and verify local Git diffs against plan goals.
    *   [Implementation Plan](plans/task_5_1_local_critic.md)
*   **Task 5.2: GitHub Webhook Listener & Active Session Event Broker**
    *   Extend the HTTP listener daemon to handle webhook notifications for issue edits, state transitions, and PR comments, routing them dynamically as reactive MCP events.
    *   [Implementation Plan](plans/task_5_2_github_webhook_listener.md)
*   **Task 5.3: Autonomous Review-Repair & Issue Comment Orchestration**
    *   Evolve the ReAct execution loop to read tasks directly from GitHub issues, run local checks, push changes, and publish progress updates and state reports back as issue comments.
    *   [Implementation Plan](plans/task_5_3_autonomous_repair_loop.md)
