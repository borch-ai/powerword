
# Powerword: The AI Capability Layer for Automated Business Pipelines

## Overview

Powerword is the **shared AI capability infrastructure** of the Borch-AI ecosystem. It is not a product — it is the engine room. Where Kiln orchestrates business decisions and Pithos runs production factories, Powerword provides the AI primitives both depend on: multi-provider LLM access, a growing library of native Go MCP plugins, and shared telemetry/cost accounting packages.

Written entirely in Go, Powerword prioritizes performance, single-binary distribution, and high-concurrency execution suitable for interactive terminal use, headless CI/CD pipelines, and subprocess invocation from orchestrators like Kiln and Pithos.

> [!NOTE]
> Powerword serves two distinct consumers: (1) **Developers** — using `powerword` as an interactive agentic CLI copilot for engineering tasks; (2) **Borch-AI pipelines** — Kiln and Pithos consume `pw-mcp-*` plugins via MCP stdio as production AI capabilities. Both use cases are first-class.

---

## Ecosystem Position

Powerword sits at the **base of the Borch-AI stack**. Everything above it is a consumer; Powerword depends on nothing within the ecosystem.

```
Kiln (intelligence + orchestration)
  └── Pithos (book factory)
        └── pw-mcp-imagegen  ─┐
            pw-mcp-seo        ├── Powerword (AI capabilities)
            pw-mcp-typst      │     ├── pkg/llm        (LLM abstraction)
            pw-mcp-trends     │     ├── pkg/telemetry  (cost accounting)
            pw-mcp-critic    ─┘     └── MCP plugin SDK
```

Powerword never imports Kiln or Pithos. They import Powerword.

---

## Core Objectives

1. **Vendor Independence**: Support Gemini, Claude, OpenAI, and local models (Ollama/vLLM) through a single `LLMClient` interface. No pipeline should be locked to a single provider.
2. **MCP Plugin Ecosystem**: Every AI capability (image generation, SEO metadata, PDF layout, market intelligence) lives as an independent `pw-mcp-*` Go binary. No capability logic belongs in Kiln or Pithos.
3. **Shared Cost Accounting**: All token consumption and API spend anywhere in the Borch-AI stack flows through `pkg/telemetry`. A single source of truth for pipeline economics.
4. **Interactive Agentic CLI**: The `powerword` binary provides a general-purpose, vendor-agnostic agentic loop for developers — separate from, but architecturally identical to, the headless plugin invocations Kiln and Pithos perform.
5. **Phase 7 Readiness**: As Kiln expands into multi-segment market intelligence, new `pw-mcp-*` plugins (`pw-mcp-trends`, `pw-mcp-typst`) must be built here before Kiln can consume them.

---

## Architecture

### 1. The Core Interactive Loop (`cmd/powerword/`)

The `powerword` CLI binary — the developer-facing agentic copilot:
- **LLM Abstraction** (`pkg/llm`): Uniform `Generate`/`Stream` interface over Gemini, Claude, OpenAI.
- **Session Management**: Persistent conversation history with checkpoint/resume capability.
- **MCP Client**: Dynamically loads and invokes `pw-mcp-*` plugins via stdio transport.
- **Output Engine**: Streaming markdown rendering with syntax highlighting in the terminal.

### 2. The MCP Plugin Library (`cmd/pw-mcp-*/`)

Each plugin is an independent Go binary implementing the MCP server protocol. Plugins are invoked as subprocesses by any MCP client (Powerword, Pithos, or Kiln directly).

**Current and planned plugins:**

| Plugin | Purpose | Primary Consumer |
|---|---|---|
| `pw-mcp-imagegen` | DALL-E 3 image generation with style references | Pithos (`brew`), Kiln (validate cover) |
| `pw-mcp-seo` | Amazon KDP keyword and A+ content generation | Kiln (`deploy`) |
| `pw-mcp-kdp-math` | Print margin, spine, bleed calculations | Pithos (`assemble`) |
| `pw-mcp-typst` | PDF layout via Typst compiler (Phase 3.11) | Pithos (`assemble`) |
| `pw-mcp-trends` | Market demand signals: Amazon Autocomplete + SerpAPI (Phase 3.10) | Kiln (`scout`, Phase 7) |
| `pw-mcp-critic` | LLM-powered diff review against implementation plans | Kiln, Pithos (pre-push hooks) |
| `pw-mcp-fs` | File system read/write | `powerword` interactive |
| `pw-mcp-git` | Git operations | `powerword` interactive |

### 3. Shared Packages (`pkg/`)

- **`pkg/llm`**: Public, imported by Pithos and (via replace directive) Kiln. The LLM abstraction interface and all provider implementations.
- **`pkg/telemetry`**: Token counting, cost tracking, and pipeline cost reporting. Shared by all tools.

---

## Technology Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26+ |
| CLI Framework | `spf13/cobra` + `spf13/viper` |
| MCP Protocol | `github.com/modelcontextprotocol/go-sdk` |
| LLM Providers | `github.com/google/generative-ai-go`, `github.com/sashabaranov/go-openai`, `github.com/anthropics/anthropic-sdk-go` |
| Config | TOML via Viper, `~/.config/powerword/config.toml` |

---

## Example Workflow

```bash
# Interactive developer use: analyze a codebase
$ powerword "Review the error handling in ./internal/forge and identify any places where context cancellation isn't respected."

# Headless pipeline use (invoked by Kiln during deploy):
$ echo '{"tool":"generate_kdp_metadata","args":{...}}' | pw-mcp-seo

# Pre-push critic hook (invoked by Kiln/Pithos git hooks):
$ pw-mcp-critic --plan plans/phase_2/task_2_1_amazon_autocomplete.md --diff <(git diff HEAD)
```

---

## What Powerword Is Not

- **Not a business orchestrator.** Kiln owns the pipeline decisions. Powerword provides the tools, not the strategy.
- **Not a book factory.** Pithos owns production. Powerword provides the AI primitives Pithos uses.
- **Not a single-vendor tool.** Every feature that works with Gemini must work with Claude and OpenAI.
- **Not a monolith.** New capabilities always ship as a new `pw-mcp-*` plugin, never as additions to the core CLI binary.

For the full ecosystem strategy, see Kiln's STRATEGY.md in the sibling Kiln repository.