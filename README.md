# Powerword

**Powerword** is a vendor-agnostic Agentic CLI written in Go. It acts as an intelligent shim that orchestrates communication between multiple Large Language Models (LLMs) and a decentralized ecosystem of tools utilizing the open Model Context Protocol (MCP).

## Overview

Unlike highly integrated, vendor-locked solutions, Powerword allows you to bring your own models (Gemini, Claude, OpenAI, or local models via Ollama) and connect them to standard MCP servers (such as local file system readers, Git integrations, or Kubernetes introspectors).

Written entirely in Go, Powerword prioritizes execution speed, straightforward single-binary distribution, and high-performance concurrency suitable for both local development workflows and automated, headless data pipelines.

## Key Features

- **Vendor Independence:** Seamlessly configure and swap between multiple LLM backends.
- **Agentic Reasoning (ReAct):** The execution loop goes beyond single-turn prompts, supporting multi-turn tool calling that allows the LLM to autonomously explore context and perform actions.
- **Standardized Extensibility:** Uses the `github.com/modelcontextprotocol/go-sdk` to manage tool lifecycles and capabilities, replacing proprietary plugin systems.
- **Secure Execution:** Designed with interactive permission guardrails before executing unsafe commands or file modifications.

## Getting Started

*(Note: Powerword is actively in development. See the Roadmap for current phase status).*

### Prerequisites
- Go 1.26.4
- Node.js (for testing standard MCP servers like `@modelcontextprotocol/server-everything`)

*Tip: A local toolchain script is available in `scripts/setup_toolchain.sh` to fetch Node.js binaries directly into the project folder without system-wide installations.*

### Build and Run

```bash
# Clone the repository
git clone https://github.com/your-org/powerword.git
cd powerword

# Build the CLI
make build

# Example execution with verbose logging enabled
POWERWORD_VERBOSE=true ./bin/powerword "Echo 'hello world' using the echo tool"
```

## Project Documentation

For a deeper dive into the architecture, technical decisions, and contribution standards, please refer to the following documents:

- **[VISION.md](VISION.md)**: High-level overview of the project vision, core objectives, and system architecture.
- **[ROADMAP.md](ROADMAP.md)**: Detailed breakdown of phases, milestones, and active implementation plans.
- **[GEMINI.md](GEMINI.md)**: Strict contribution guidelines, coding standards, and autonomous AI agent instructions.
- **[plans/](plans/)**: Detailed, step-by-step implementation plans for all active roadmap tasks.

## License

[MIT License](LICENSE)
