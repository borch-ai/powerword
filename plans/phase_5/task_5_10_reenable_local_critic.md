# plan: Task 5.10: Re-enable Local Critic (Ollama)

**Status:** Open
**Go Version:** [Go Version Placeholder]
**Date Completed:** [Date Completed Placeholder]
**Unit Test Coverage:** [Unit Test Coverage Placeholder]

This plan outlines the steps to re-enable the local critic LLM reviews by configuring and running a local offline LLM (such as Llama 3 or Mistral) via Ollama, once the hardware is prepared.

## User Review Required

None.

## Proposed Changes

### Configuration

#### [MODIFY] [powerword.toml](file://../../powerword.toml)

- Set `disable_critic = false` (or remove the line to default to false).
- Configure the critic server options to use the local Ollama instance:

  ```toml
  critic_provider = "ollama"
  critic_endpoint = "http://localhost:11434/v1"
  critic_model = "llama3" # or another model of choice (e.g., mistral, phi3)
  ```

---

## Verification Plan

### Manual Verification

- Start Ollama locally and pull the target model (e.g. `ollama run llama3`).
- Run `powerword review --local` and verify that the pre-push local review runs the critique via Ollama and outputs `VERDICT: ACCEPT` without making any external API calls.
