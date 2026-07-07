# Powerword Roadmap

This roadmap defines the engineering journey to build **Powerword**, the vendor-agnostic Agentic CLI. It breaks down the system design into logical phases and actionable tasks, each accompanied by a detailed implementation plan.

---

## Ecosystem Context

Powerword is the **infrastructure layer** of the Borch-AI publishing stack. It provides the MCP plugin network and shared Go packages (`pkg/llm`, `pkg/config`, `pkg/telemetry`) that all sibling projects consume.

- **Pithos** imports `pkg/llm`, `pkg/telemetry`; invokes `pw-mcp-imagegen`, `pw-mcp-seo`, `pw-mcp-kdp-math` binaries.
- **Kiln** invokes `pw-mcp-seo`, `pw-mcp-imagegen` directly; drives Pithos as a subprocess.
- **Lamplighter** — future consumer of `pw-mcp-telemetry` for real-time cost dashboards.
- **Lighthouse** — the telemetry sink. `pkg/telemetry` will optionally forward session metrics to Lighthouse's `POST /api/telemetry` on command exit (opt-in via `LIGHTHOUSE_URL`).

Powerword **does not depend on** any sibling project. Additions to this roadmap that are clearly domain-specific to publishing or kiln orchestration should be implemented here as MCP servers, then *consumed* by the appropriate sibling.

---

## Phase 1: Foundation
Focus: Bootstrapping the CLI application, establishing the LLM interface layer, and building a basic prompt-response stream.

*   [x] **Task 1.1: Project Initialization & Cobra/Viper Configuration**
    *   Set up the Go project structure, module, and command-line parsing structure with Cobra. Configure global settings (API keys, defaults) loaded via Viper.
    *   [Implementation Plan](plans/phase_1/task_1_1_cobra_viper_setup.md)
*   [x] **Task 1.2: LLM Abstraction Layer**
    *   Create a unified Go interface for interacting with LLM providers. Implement concrete providers for Gemini and OpenAI/Claude.
    *   [Implementation Plan](plans/phase_1/task_1_2_llm_abstraction.md)
*   [x] **Task 1.3: Core Loop & Streaming Output Engine**
    *   Implement the primary execution loop to pass user prompts to the abstraction layer, capture the stream, and render syntax-highlighted markdown back to the terminal.
    *   [Implementation Plan](plans/phase_1/task_1_3_core_loop.md)
*   [x] **Task 1.4: Session Persistence & Chat History Management**
    *   Implement a local persistence engine (JSON files or SQLite database) in `~/.local/share/powerword/sessions` to store chat history, session state, and model parameters. Support resuming past sessions using a `--session` CLI flag.
    *   [Implementation Plan](plans/phase_1/task_1_4_session_persistence.md)
*   [x] **Task 1.5: Markdown Linting & Implementation Plan Validation**
    *   Integrate Markdown linting tooling (`markdownlint-cli`) into the repository gates. Establish an automated structure check script to verify that all implementation plans conform to the core template standard.
    *   [Implementation Plan](plans/phase_1/task_1_5_markdown_linting.md)
*   [x] **Task 1.8: Local Toolchain Sandbox**
    *   Create a local, sandboxed script to fetch and run Node.js locally without any system-wide packages or containers.
    *   [Implementation Plan](plans/phase_1/task_1_8_local_toolchain.md)

---

## Phase 2: Protocol Integration
Focus: Integrating the Model Context Protocol (MCP) and adapting the execution loop to orchestrate tools.

*   [x] **Task 2.1: MCP Go SDK Integration**
    *   Integrate `modelcontextprotocol/go-sdk` as a library dependency. Structure the internal client code to discover and map server capabilities.
    *   [Implementation Plan](plans/phase_2/task_2_1_mcp_integration.md)
*   [x] **Task 2.2: Stdio Transport Layer & Server Lifecycle**
    *   Build standard I/O (stdio) transport handlers to launch, monitor, and clean up external MCP servers running as child processes.
    *   [Implementation Plan](plans/phase_2/task_2_2_stdio_transport.md)
*   [x] **Task 2.3: Tool Calling Execution Loop**
    *   Evolve the execution loop from standard stream to a multi-turn ReAct reasoning loop. Convert LLM tool calls to MCP requests, run the tools, and return execution results back to the LLM.
    *   [Implementation Plan](plans/phase_2/task_2_3_tool_calling_loop.md)
*   [x] **Task 2.4: Interactive Permission & Consent Manager**
    *   Build a CLI permission prompt engine to intercept tool executions (like filesystem edits, commands, or network hits). Prompt the user interactively in the terminal before running unsafe tools.
    *   [Implementation Plan](plans/phase_2/task_2_4_permission_manager.md)

---

## Phase 3: The Go Plugin Ecosystem & Systems Integration
Focus: Delivering a standard set of native, high-performance Go MCP servers, a configuration schema, and specialized operations tools.

*   [x] **Task 3.1: Standard Native Plugins (FS, Git, Shell)**
    *   Author a suite of lightweight Go-based MCP servers for reading local files, introspecting Git repositories, and executing shell commands with strict security profiles.
    *   [Implementation Plan](plans/phase_3/task_3_1_go_plugins.md)
*   [x] **Task 3.2: Plugin Manifest Configuration & Registration**
    *   Create a YAML-based plugins manifest schema. Enable the CLI to parse user configuration files to mount and spin up custom local MCP servers on launch.
    *   [Implementation Plan](plans/phase_3/task_3_2_plugin_manifest.md)
*   [ ] **Task 3.3: Kubernetes Diagnostician Plugin (K8s)**
    *   Implement a native Go MCP server to communicate with local/remote Kubernetes clusters to inspect namespaces, check service states, dump failing pod logs, and run diagnostics.
    *   [Implementation Plan](plans/phase_3/task_3_3_k8s_mcp.md)
*   [ ] **Task 3.4: SQL Database Inspector Plugin**
    *   Build a database bridge Go MCP server supporting Postgres, MySQL, and SQLite. Support listing schemas, describing table structures, executing read-only check queries, and alerting on locks or long-running queries.
    *   [Implementation Plan](plans/phase_3/task_3_4_db_mcp.md)
*   [x] **Task 3.5: Creative Asset Generation Plugin (ImageGen)**
    *   Implement a native Go MCP server supporting OpenAI's DALL-E 3 and custom Midjourney wrappers to generate, catalog, and query consistent style referenced images.
    *   [Implementation Plan](plans/phase_3/task_3_5_imagegen_mcp.md)
*   [x] **Task 3.6: KDP Book Geometry & PDF Validator Plugin (KDP Math)**
    *   Implement a native Go MCP server to calculate exact cover, interior, margins, and bleed specs for Amazon KDP print books, and run validation on final compiled PDF page dimensions.
    *   [Implementation Plan](plans/phase_3/task_3_6_kdp_math_mcp.md)
*   [x] **Task 3.7: Viral Promo Asset Builder Plugin (Viral)**
    *   Implement a native Go MCP server wrapping TTS engines (ElevenLabs) and generative video tools to synthesize ASMR narration and stitch promotional trailers using local ffmpeg commands.
    *   [Implementation Plan](plans/phase_3/task_3_7_viral_mcp.md)
*   [x] **Task 3.8: Amazon KDP SEO & Metadata Agent Plugin (SEO)**
    *   Implement a native Go MCP server querying keyword volumes and product search suggestions to formulate listing titles, descriptions, and tag payloads.
    *   [Implementation Plan](plans/phase_3/task_3_8_seo_mcp.md)
*   [ ] **Task 3.9: Google Doc MCP Plugin (GDoc)**
    *   Implement a native Go MCP server to create, read, and update Google Docs to export manuscripts for editing and import them back.
    *   [Implementation Plan](plans/phase_3/task_3_9_gdoc_mcp.md)
*   [x] **Task 3.10: Market Intelligence Plugin (`pw-mcp-trends`)**
    *   Implement a native Go MCP server wrapping Amazon Autocomplete (free, unauthenticated) and SerpAPI Google Trends to return ranked niche keyword candidates with demand velocity scores. Primary consumer: the Kiln `scout` engine (Task 2.2). Defines the `TrendSource` interface so additional backends (Reddit, TikTok) can be injected without changing the MCP surface. Blocks Kiln Task 2.2.
    *   [Implementation Plan](plans/phase_3/task_3_10_trends_mcp.md)
*   [x] **Task 3.11: Typst PDF Layout Plugin (`pw-mcp-typst`)**
    *   Implement a native Go MCP server that invokes a local Typst binary to compile book manuscripts and illustration assets into print-ready PDFs conforming to KDP bleed/margin specs. Primary consumer: the Pithos `assemble` engine (Task 4.2). This unblocks Pithos Phase 4 which is currently stalled waiting for this server.
    *   [Implementation Plan](plans/phase_3/task_3_11_typst_mcp.md)
*   [x] **Task 3.12: EPUB Publication Builder (`pw-mcp-epub`)**
    *   Implement a native Go MCP server that compiles parodic manuscripts and illustration assets into spec-compliant EPUB digital publications. Primary consumer: Pithos (`digital-export`).
    *   [Implementation Plan](plans/phase_3/task_3_12_epub_mcp.md)
*   [x] **Task 3.13: Print-Ready PDF Preflight Inspector (`pw-mcp-pdfcheck`)**
    *   Implement a native Go MCP server to perform deep validation of compiled PDF book geometry, bleed limits, embedded fonts, and image resolution (minimum 300 DPI) against Amazon KDP paperback ingest rules. Primary consumer: Pithos (`assemble`).
    *   [Implementation Plan](plans/phase_3/task_3_13_pdfcheck_mcp.md)
*   [x] **Task 3.14: Typst Verse Line Break Formatting (`pw-mcp-typst`)**
    *   Preserve line breaks inside parsed Markdown stanzas and replace them with Typst-compliant break markers (` \\\n `) before compiling the PDF.
    *   [Implementation Plan](plans/phase_3/task_3_14_typst_line_breaks.md)
*   [x] **Task 3.15: Typst Mixed Layout Templates (`pw-mcp-typst`)**
    *   Extend `pw-mcp-typst` compile tools to support left-hand text, right-hand image, and other layout styles instead of only full-bleed backgrounds.
    *   [Implementation Plan](plans/phase_3/task_3_15_typst_mixed_layouts.md)
*   [x] **Task 3.16: Slideshow Video Trailer Stitching (`pw-mcp-viral`)**
    *   Extend the `pw-mcp-viral` toolset with a slideshow generator tool (`viral_stitch_slideshow`) that compiles sequences of paired images and audio narration segments into a single unified MP4 video trailer using ffmpeg.
    *   [Implementation Plan](plans/phase_3/task_3_16_viral_slideshow.md)
*   [x] **Task 3.17: Grayscale PDF Preflight Checker (`pw-mcp-pdfcheck`)**
    *   Extend the `pw-mcp-pdfcheck` toolset with grayscale enforcement to verify that manuscripts intended for black-and-white print editions only contain grayscale elements (no RGB or CMYK coordinates/color profiles).
    *   [Implementation Plan](plans/phase_3/task_3_17_pdfcheck_grayscale.md)
*   [x] **Task 3.18: Gutter & Safe-Zone Margin Analyzer (`pw-mcp-pdfcheck`)**
    *   Implement dynamic gutter and page margin safe-zone checks in `pw-mcp-pdfcheck` to verify that text and structural elements do not run too close to the binding gutter or outer page trim boundaries.
    *   [Implementation Plan](plans/phase_3/task_3_18_pdfcheck_margins.md)
*   [x] **Task 3.19: Spine Cover Wrap & Barcode Layout Checker (`pw-mcp-pdfcheck`)**
    *   Implement cover wrap geometry checking and back cover barcode detection in `pw-mcp-pdfcheck` to verify that compiled book covers have the exact KDP-compliant dimensions and contain a readable barcode.
    *   [Implementation Plan](plans/phase_3/task_3_19_pdfcheck_cover_barcode.md)
*   [x] **Task 3.20: Total Area Coverage Ink Density Analyzer (`pw-mcp-pdfcheck`)**
    *   Implement Total Area Coverage (TAC) ink limit auditing in `pw-mcp-pdfcheck` to check that print-ready PDF files do not exceed maximum ink density limits (e.g. 240% for KDP paperbacks), preventing smudging and press tearing.
    *   [Implementation Plan](plans/phase_3/task_3_20_pdfcheck_ink_coverage.md)
*   [x] **Task 3.21: Typst Per-Page Layout Overrides (`pw-mcp-typst`)**
    *   Extend `pw-mcp-typst`'s `ParseManuscript` parser and template compilation to support per-page layout configuration via comments (e.g. `<!-- Layout: <type> -->`) inside the Markdown manuscript.
    *   [Implementation Plan](plans/phase_3/task_3_21_typst_per_page_layouts.md)
*   [x] **Task 3.22: YouTube Integration Plugin (`pw-mcp-youtube`)**
    *   Implement a native Go MCP server exposing tools to interface with YouTube APIs (video uploads, metadata customization, and analytics fetching).
    *   [Implementation Plan](plans/phase_3/task_3_22_youtube_mcp.md)
*   [ ] **Task 3.23: Digital Music Distribution Helper (`pw-mcp-music`)**
    *   Implement a native Go MCP server exposing tools to package lossless audio and artwork and submit releases to distributor webhook endpoints.
    *   [Implementation Plan](plans/phase_3/task_3_23_music_mcp.md)

---

## Phase 4: Operations Automation & Advanced Features
Focus: Enhancing coordinator routing, telemetry, cloud integration, and non-interactive execution engines.

*   [x] **Task 4.1: Multi-Model Orchestration & Intelligent Routing**
    *   Create a routing component to allocate tasks dynamically. For example, route simple context checks to smaller local/fast models, reserving large reasoning models for complex tool orchestration.
    *   [Implementation Plan](plans/phase_4/task_4_1_multi_model.md)
*   [x] **Task 4.2: Headless Pipelines & Automation**
    *   Support non-interactive pipeline execution modes. Allow Powerword to digest raw stdin stream arguments, output structured JSON format, and behave as a reliable utility in CI/CD environments.
    *   [Implementation Plan](plans/phase_4/task_4_2_headless_pipeline.md)
*   [x] **Task 4.3: Telemetry, Token Metrics & Cost Accounting**
    *   Implement usage accounting to track input, output, and cached tokens consumed during loops. Calculate and display cost metrics on execution exit.
    *   [Implementation Plan](plans/phase_4/task_4_3_telemetry_cost.md)
*   [x] **Task 4.4: Cloud Orchestrator Plugin (AWS/GCP)**
    *   Create a lightweight native Go MCP server (`pw-mcp-cloud`) to parse cloud console resource metadata (EC2/GCE states, CloudWatch logs, storage buckets) to query deployment status.
    *   [Implementation Plan](plans/phase_4/task_4_4_cloud_mcp.md)
*   [x] **Task 4.5: Shareable Telemetry Subpackage Refactor**
    *   Refactor the telemetry and token cost accounting logic from `internal/llm/telemetry.go` to a dependency-free public package `pkg/telemetry`.
    *   Change the module name of `powerword` to `github.com/borch-ai/powerword` so it is importable.
    *   [Implementation Plan](plans/phase_4/task_4_5_telemetry_refactor.md)
*   [x] **Task 4.6: Cloud Storage Subsystem**
    *   Implement a cloud storage uploader engine within `pw-mcp-cloud` and expose it as a `cloud_upload_file` MCP tool supporting Firebase/GCS and S3 uploads.
    *   [Implementation Plan](plans/phase_4/task_4_6_cloud_storage.md)
*   [x] **Task 4.7: MCP ImageGen Cref Extensions**
    *   Extend the `pw-mcp-imagegen` server to accept `cref_url` and `character_weight` inputs. Automatically upload local character assets before generation.
    *   [Implementation Plan](plans/phase_4/task_4_7_imagegen_cref_extensions.md)
*   [x] **Task 4.8: Google Cloud Run Orchestration Tools (`pw-mcp-cloud`)**
    *   Extend `pw-mcp-cloud` to support serverless container deployments. Expose tools like `cloud_deploy_run_service`, `cloud_get_run_service`, and `cloud_list_run_services` to control Google Cloud Run services.
    *   [Implementation Plan](plans/phase_4/task_4_8_cloud_run_orchestration.md)
*   [ ] **Task 4.9: Artifact Registry & Container Delivery Verification (`pw-mcp-cloud`)**
    *   Implement tools to inspect Google Artifact Registry, checking for container image tags and delivery status via `cloud_check_registry_image` before deploying to Cloud Run.
    *   [Implementation Plan](plans/phase_4/task_4_9_artifact_registry.md)
*   [ ] **Task 4.10: GCP Secret Manager Integration (`pw-mcp-cloud`)**
    *   Expose tools to manage and retrieve runtime secrets securely from GCP Secret Manager (such as database credentials and API keys) to avoid exposing credentials.
    *   [Implementation Plan](plans/phase_4/task_4_10_secret_manager.md)
*   [ ] **Task 4.11: IAM Policies & Pre-flight API Verification (`pw-mcp-cloud`)**
    *   Add validation tools `cloud_verify_iam_roles` and `cloud_check_enabled_apis` to verify service accounts have permissions and that target GCP APIs are enabled.
    *   [Implementation Plan](plans/phase_4/task_4_11_iam_api_verification.md)
*   [x] **Task 4.12: Google Veo & Imagen Character Seeding Support**
    *   Extend the `pw-mcp-imagegen` server's Google Veo and Imagen backends to support image-based references and character consistency. Route `crefURL` and `characterWeight` to the Google GenAI API where applicable (e.g. as a starting image frame for Veo or style reference/image-to-image input), or handle fallback consistency adjustments.
    *   [Implementation Plan](plans/phase_4/task_4_12_veo_character_reference_support.md)
*   [x] **Task 4.13: ImageGen Capabilities Handshake and Validation**
    *   Implement an `imagegen_get_capabilities` tool in the `pw-mcp-imagegen` server to expose active backend parameters (e.g. supports_cref). Validate generation arguments and return tool errors when unsupported options are passed.
    *   [Implementation Plan](plans/phase_4/task_4_13_imagegen_capabilities_validation.md)
*   [x] **Task 4.14: Backend-Owned Imagegen Capabilities Mapping & Overrides**
    *   Add a `Capabilities()` method to each imagegen backend struct so each backend type owns and reports its own capability set. Refactor `ImageGenService.GetCapabilities()` to delegate to the active backend type. Add `force_cref`/`force_sref` config overrides (with env var bindings) to bypass capability checks when upstream models change.
    *   [Implementation Plan](plans/phase_4/task_4_14_imagegen_model_capabilities.md)
*   [x] **Task 4.15: Imagegen Capabilities Output Type Field**
    *   Extend the image generation capabilities schema to include a new `output_type` field indicating whether a backend generates still images or videos. Update the Google Imagen, DALL-E, Midjourney, and Google Veo backends to report their respective output types.
    *   [Implementation Plan](plans/phase_4/task_4_15_imagegen_capabilities_output_type.md)
*   [ ] **Task 4.16: Lighthouse Telemetry HTTP Adapter (`pkg/telemetry`)**
    *   Extend `pkg/telemetry` with an optional HTTP adapter that submits session metrics (project, command, stage, duration\_ms, cost\_usd, tokens\_in, tokens\_out, tokens\_cached) to a Lighthouse `POST /api/telemetry` endpoint on command exit. Enabled via `LIGHTHOUSE_URL` environment variable — no-ops silently if unset so no existing callers break. The adapter runs in a goroutine with a short deadline to avoid blocking the CLI exit. All sibling tools (Pithos, Aeolian, Kiln) that import `pkg/telemetry` get this capability for free once the adapter is merged. Blocked on Lighthouse Task 1.4.
    *   [Implementation Plan](plans/phase_4/task_4_16_lighthouse_telemetry_adapter.md)



---

## Phase 5: Autonomous Review & Repair Loop
Focus: Delivering a fully autonomous local-to-remote review feedback and code correction pipeline driven by GitHub Issues.

*   [x] **Task 5.1: Structured Issue Templates & Local Critic Integration**
    *   Define GitHub YAML Issue Forms to enforce structured planning schemas, and implement a `powerword review` sub-command to parse active issue fields and verify local Git diffs against plan goals.
    *   [Implementation Plan](plans/phase_5/task_5_1_local_critic.md)
*   [x] **Task 5.2: GitHub Webhook Listener & Active Session Event Broker**
    *   Extend the HTTP listener daemon to handle webhook notifications for issue edits, state transitions, and PR comments, routing them dynamically as reactive MCP events.
    *   [Implementation Plan](plans/phase_5/task_5_2_github_webhook_listener.md)
*   [x] **Task 5.3: Autonomous Review-Repair & Issue Comment Orchestration**
    *   Evolve the ReAct execution loop to read tasks directly from GitHub issues, run local checks, push changes, and publish progress updates and state reports back as issue comments.
    *   [Implementation Plan](plans/phase_5/task_5_3_autonomous_repair_loop.md)
*   [ ] **Task 5.4: MCP Telemetry Migration**
    *   Extract local telemetry calculations into a standalone `pw-mcp-telemetry` MCP server, and refactor Powerword to query it over standard I/O.
    *   [Implementation Plan](plans/phase_5/task_5_4_mcp_telemetry_migration.md)
*   [x] **Task 5.5: Plan Conformance & Validation Integration**
    *   Integrate plan template and relative link conformance checks directly into the `powerword review` command. Parse local plan templates and enforce required status metadata blocks (Go Version, Date Completed, Unit Test Coverage) and link checks as an automated pre-review validation gate.
    *   [Implementation Plan](plans/phase_5/task_5_5_plan_validation.md)
*   [x] **Task 5.6: Test Coverage Check Command**
    *   Add a project-agnostic `powerword check-coverage` subcommand. Parse standard Go coverage output profiles and compare overall statement coverage percentages against specified minimum thresholds, failing with a non-zero exit status if threshold is unmet.
    *   [Implementation Plan](plans/phase_5/task_5_6_check_coverage.md)
*   [x] **Task 5.7: Multi-Language Coverage Plugin (LCOV & Cobertura)**
    *   Implement a standalone `pw-mcp-coverage` MCP server to support parsing LCOV (`lcov.info`) and Cobertura (`coverage.xml`) formats. Extend the `powerword check-coverage` command to call this server, enabling automated coverage gates for TypeScript/Vitest (such as in Knurl) and Python codebases.
    *   [Implementation Plan](plans/phase_5/task_5_7_multilang_coverage.md)
*   [x] **Task 5.8: Absolute Link Verification & Auto-Fixing**
    *   Enhance the plan validator to catch absolute filepaths and add an auto-fixing option to convert absolute path links into relative workspace links automatically.
    *   [Implementation Plan](plans/phase_5/task_5_8_absolute_link_validation.md)
*   [x] **Task 5.9: Opt-In Critic LLM Reviews**
    *   Introduce a configuration setting to run the LLM-powered review step in the workspace critic as an opt-in feature, while preserving local plan validations and local build/test checks by default.
    *   [Implementation Plan](plans/phase_5/task_5_9_disable_critic.md)
*   [ ] **Task 5.10: Re-enable Local Critic (Ollama)**
    *   Re-enable the local critic LLM reviews configured with a local model run offline via Ollama, once the hardware is prepared.
    *   [Implementation Plan](plans/phase_5/task_5_10_reenable_local_critic.md)
*   [x] **Task 5.11: Allow `review --fix` Without an API Key**
    *   `powerword review --fix` auto-corrects plan file formatting but currently fails with "no API keys found" even though no LLM call is made. Fix `persistentPreRunE` to skip `cfg.Validate()` for fix-only invocations; move API key validation into each subcommand's `RunE` at the point where LLM access is actually needed.
    *   [Implementation Plan](plans/phase_5/task_5_11_fix_only_no_api_key.md)

---

## Phase 6: Enterprise Security, Safety & Developer Experience
Focus: Advancing agent safety guardrails, remote transport protocols, robust sandboxing, and terminal graphics.

*   [x] **Task 6.1: Git-Backed Workspace Rollbacks**
    *   Implement workspace snapshots and rollback mechanics to restore clean working states if an autonomous agent fails run/compile steps.
    *   [Implementation Plan](plans/phase_6/task_6_1_workspace_rollbacks.md)
*   [x] **Task 6.2: Token & Cost Budgeting Guardrails**
    *   Add user-defined dollar and token budget safety valves per session or loop to prevent runaway API spend.
    *   [Implementation Plan](plans/phase_6/task_6_2_token_budgets.md)
*   [x] **Task 6.3: Pause & Resume Session States**
    *   Introduce mechanisms to serialize execution frames, enabling manual user fixes before resuming a paused agent session.
    *   [Implementation Plan](plans/phase_6/task_6_3_session_resumability.md)
*   [ ] **Task 6.4: Granular Tool Access Profiles**
    *   Create white/blacklist filters and path restrictions to limit filesystem and command execution scopes.
    *   [Implementation Plan](plans/phase_6/task_6_4_tool_access_profiles.md)
*   [ ] **Task 6.5: WASM-Based Plugin Sandboxing**
    *   Support running MCP servers compiled to WebAssembly (WASM) to isolate plugin code from host resources.
    *   [Implementation Plan](plans/phase_6/task_6_5_wasm_sandboxing.md)
*   [ ] **Task 6.6: Dry-Run Mode for Operations Plugins**
    *   Implement non-destructive validation passes for Kubernetes, SQL, and Cloud orchestrators to check actions before execution.
    *   [Implementation Plan](plans/phase_6/task_6_6_operations_dry_run.md)
*   [ ] **Task 6.7: Remote SSE & WebSocket Client Transport**
    *   Extend the MCP transport layer to connect to remote plugins running over Server-Sent Events (SSE) or WebSockets with TLS/Auth.
    *   [Implementation Plan](plans/phase_6/task_6_7_remote_transport.md)
*   [ ] **Task 6.8: Inline Graphics Rendering**
    *   Integrate Kitty, iTerm2, and Sixel protocols to render generated graphics (images/plots) directly within supported terminal sessions.
    *   [Implementation Plan](plans/phase_6/task_6_8_terminal_graphics.md)
*   [ ] **Task 6.9: Rich Interactive Consent TUI & Remote Approvals**
    *   Replace standard shell prompts with a Bubbletea-based terminal interface detailing tool calls and risk profiles. Integrate Firebase RTDB remote consent signaling `/tunnels/{tunnelId}/approval` to support mobile biometric approval handshakes from Lamplighter.
    *   [Implementation Plan](plans/phase_6/task_6_9_interactive_consent_tui.md)
*   [ ] **Task 6.10: Headless JSON Envelopes & PR Review Mode**
    *   Standardize structured JSON outputs for CI integration and build webhooks to orchestrate inline PR comment review loops.
    *   [Implementation Plan](plans/phase_6/task_6_10_headless_ci.md)
*   [x] **Task 6.11: Generalized MCP Critic Server**
    *   Refactor the existing Powerword Local Critic subsystem into a standalone, generalized Model Context Protocol (MCP) server (pw-mcp-critic) to be shared across projects.
    *   [Implementation Plan](plans/phase_6/task_6_11_critic_mcp.md)
*   [x] **Task 6.12: Standalone Linter Suite (CLI, `pw-mcp-linter`)**
    *   Promote `internal/linter` to `pkg/linter` (public). Add `powerword lint-plans` and `powerword lint-go` subcommands. Build `pw-mcp-linter` as a standalone MCP server exposing a `lint_plans` tool that returns structured JSON errors, allowing autonomous agents to run cheap pre-flight plan checks.
    *   [Implementation Plan](plans/phase_6/task_6_12_standalone_plan_linter.md)
*   [ ] **Task 6.13: Isolated Execution via Git Worktrees**
    *   Introduce support for running agent loops in a completely isolated Git worktree, including copying/mounting uncommitted changes and selectively symlinking caches/dependencies to speed up builds.
    *   [Implementation Plan](plans/phase_6/task_6_13_isolated_worktrees.md)
*   [ ] **Task 6.14: Publish Automated Binary Releases of MCP Plugins**
    *   Extend the release CI workflow to compile and attach all MCP plugin binaries (`pw-mcp-fs`, `pw-mcp-git`, `pw-mcp-shell`, `pw-mcp-imagegen`, `pw-mcp-kdp-math`, `pw-mcp-seo`, `pw-mcp-viral`, `pw-mcp-critic`, `pw-mcp-epub`, `pw-mcp-pdfcheck`, `pw-mcp-linter`) to GitHub Releases.
    *   [Implementation Plan](plans/phase_6/task_6_14_publish_mcp_releases.md)
*   [x] **Task 6.15: End-to-End Pipeline & MCP Integration Testing Suite**
    *   Implement a dedicated integration test suite using build tags (`//go:build integration`) to test the compiled binary CLI workflows, session file operations, and native stdio MCP plugin transport handshakes.
    *   [Implementation Plan](plans/phase_6/task_6_15_integration_tests.md)
*   [x] **Task 6.16: LLM JSON Mode & Client Options**
    *   Refactor `pkg/llm` in Powerword to support structured JSON generation via response MIME-type parameters, and ensure clients can be easily instantiated without tight coupling to Powerword's internal config.
    *   [Implementation Plan](plans/phase_6/task_6_16_llm_json_mode.md)
*   [x] **Task 6.17: Shared LLM Structured Outputs & Schema Enforcement**
    *   Enhance `pkg/llm` in Powerword to support first-class structured/schema constraints (e.g., OpenAI Structured Outputs and Gemini response schema). Provide standard option builders and type-safe translations in all client adapters.
    *   [Implementation Plan](plans/phase_6/task_6_17_llm_structured_outputs.md)
*   [ ] **Task 6.18: Speculative — Pithos Pipeline MCP Server (`pw-mcp-pithos`)**
    *   Wrap the Pithos book production pipeline behind a formal MCP server, enabling Kiln and Lamplighter to invoke and monitor `initiate`, `brew`, `assemble`, and `deploy` stages via standard MCP protocol instead of raw subprocess calls. This is the long-term upgrade path for Kiln's forge integration (Kiln Phase 4 → Phase 6 migration). Not to be built until Pithos `deploy` is fully implemented.
    *   [Implementation Plan](plans/phase_6/task_6_18_pithos_mcp_server.md)
*   [x] **Task 6.19: Market Intelligence Plugin (`pw-mcp-trends`)**
    *   Implement a native Go MCP server wrapping Amazon Autocomplete (free, unauthenticated) and SerpAPI Google Trends to return ranked niche keyword candidates with demand velocity scores. Primary consumer: the Kiln `scout` engine. Defines the `TrendSource` interface so additional backends (Reddit, TikTok) can be injected without changing the MCP surface.
    *   [Implementation Plan](plans/phase_6/task_6_19_trends_mcp.md)
*   [x] **Task 6.20: Shared Git Utility Package (`pkg/gitutil`)**
    *   Extract the Git command execution logic from internal rollback utilities into a reusable, public package `pkg/gitutil` for use across sibling projects.
    *   [Implementation Plan](plans/phase_6/task_6_20_shared_git_utility.md)
*   [x] **Task 6.21: Provider API Key Fallback Resolution**
    *   Add fallback resolution to `LoadConfig` so that canonical provider env var names (`GEMINI_API_KEY`, `GOOGLE_API_KEY`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `SERP_API_KEY`) are recognised when the `POWERWORD_`-prefixed variant is not set. Eliminates the need to set separate keys per tool in the Borch-AI stack — one key covers the whole stack. `POWERWORD_*` vars retain highest precedence.
    *   [Implementation Plan](plans/phase_6/task_6_21_api_key_fallback_resolution.md)
*   [x] **Task 6.22: Configurable & Generalized Image/Video Generation Timeouts**
    *   Add a configurable `plugins.imagegen.request_timeout` parameter (defaulting to 120s) across all backends (Google Imagen, Google Veo, Midjourney, OpenAI DALL-E) and download helpers to prevent context deadline exceeded issues under high load.
    *   [Implementation Plan](plans/phase_6/task_6_22_configurable_imagegen_timeouts.md)
*   [ ] **Task 6.23: Standardized Request Retry Engine with Exponential Backoff**
    *   Implement transient error detection and auto-retries with exponential backoff and jitter for GenAI API requests and image generators.
    *   [Implementation Plan](plans/phase_6/task_6_23_retry_backoff.md)
*   [ ] **Task 6.24: Active Process Termination & Poll Cleanup on Context Cancellation**
    *   Ensure all native plugins listen to context cancellation and actively kill spawned subprocesses and terminate background API polling.
    *   [Implementation Plan](plans/phase_6/task_6_24_process_termination.md)
*   [ ] **Task 6.25: OpenTelemetry (OTel) Tracing Instrumentation**
    *   Add trace spans to the ReAct reasoning loop and tool calling interface to monitor execution latency and trace tool interactions.
    *   [Implementation Plan](plans/phase_6/task_6_25_opentelemetry_tracing.md)
*   [x] **Task 6.26: Centralized Pull Request Issue Linker Subcommand & GitHub Adaptations**
    *   Implement a native Go `powerword link-issue` subcommand to scan plan diffs and auto-link issue references to PR descriptions. Adopt and customize `.github` templates (Copilot instructions, pull request template, dependabot checks) from Aeolian.
    *   [Implementation Plan](plans/phase_6/task_6_26_link_issue_subcommand.md)
*   [x] **Task 6.27: Reduce `//nolint` Suppressions to Absolute Minimum**
    *   Systematically eliminate the 185 `//nolint` tags in the codebase through two strategies: (1) consolidating repeated, identically-justified `gosec` suppressions into single documented helper functions (`safeReadFile`, `runFFmpeg`, `newTrustedRequest`, `LoadFromWorkspace`), and (2) decomposing the large functions that carry `gocognit`, `funlen`, `nestif`, and `gocyclo` suppressions into smaller, independently unit-testable helpers. Also audits and removes stale/redundant suppressions.
    *   [Implementation Plan](plans/phase_6/task_6_27_nolint_reduction.md)
*   [x] **Task 6.28: Advanced Integration & Subprocess Testing Suite Expansion**
    *   Expand the integration test suite to include subprocess stdio tests for the remaining MCP servers (`pw-mcp-critic`, `pw-mcp-linter`, `pw-mcp-kdp-math`, `pw-mcp-seo`). Add compiled CLI tests for `powerword review` and `powerword link-issue` subcommands using mock stub dependencies.
    *   [Implementation Plan](plans/phase_6/task_6_28_integration_tests_expansion.md)









