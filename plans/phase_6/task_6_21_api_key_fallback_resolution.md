# plan: Task 6.21: Provider API Key Fallback Resolution

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

---

## User Review Required

> [!NOTE]
> No breaking changes. Existing POWERWORD_* env vars retain highest precedence. Fully backward-compatible.

---

## Problem

Every tool in the Borch-AI stack (Powerword, Pithos, Kiln) ultimately routes LLM calls through Powerword. The stack-wide credential vars are therefore `POWERWORD_GEMINI_API_KEY`, `POWERWORD_ANTHROPIC_API_KEY`, and `POWERWORD_OPENAI_API_KEY`.

However, a developer setting up the stack for the first time will likely try the provider's own canonical env var names first — the names documented by Google, Anthropic, and OpenAI themselves:

| Provider | Canonical SDK var | Powerword var |
|---|---|---|
| Google Gemini | `GEMINI_API_KEY` or `GOOGLE_API_KEY` | `POWERWORD_GEMINI_API_KEY` |
| Anthropic | `ANTHROPIC_API_KEY` | `POWERWORD_ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` | `POWERWORD_OPENAI_API_KEY` |
| SerpAPI | `SERP_API_KEY` | `POWERWORD_SERP_API_KEY` |

Today, setting `GEMINI_API_KEY` has no effect — Powerword ignores it entirely. This causes confusing "no API keys found" errors when the key is clearly present in the environment under a slightly different name.

---

## Goal

Implement **fallback resolution** so that provider canonical var names are recognised when the `POWERWORD_*` prefixed var is not set. The resolution order for each key is:

```
api_keys.gemini:
  1. POWERWORD_GEMINI_API_KEY    (tool-specific, highest precedence)
  2. GEMINI_API_KEY              (Google AI Studio canonical)
  3. GOOGLE_API_KEY              (Google Cloud / Vertex canonical)

api_keys.openai:
  1. POWERWORD_OPENAI_API_KEY
  2. OPENAI_API_KEY              (OpenAI SDK canonical)

api_keys.anthropic:
  1. POWERWORD_ANTHROPIC_API_KEY
  2. ANTHROPIC_API_KEY           (Anthropic SDK canonical)

plugins.trends.serp_api_key:
  1. POWERWORD_SERP_API_KEY
  2. SERP_API_KEY                (SerpAPI canonical)
```

**OS-level env vars always take precedence over `.env` file values.** The fallback chain applies equally to both sources.

---

## Proposed Changes

### `pkg/config/config.go`

#### Update `bindEnv` calls for the four API key fields

Viper's `BindEnv` accepts multiple env var names and uses the first one that is set. Replace the current single-var bindings with multi-var bindings:

```go
// Before:
bindEnv(v, "api_keys.gemini", "POWERWORD_GEMINI_API_KEY")
bindEnv(v, "api_keys.openai", "POWERWORD_OPENAI_API_KEY")
bindEnv(v, "api_keys.anthropic", "POWERWORD_ANTHROPIC_API_KEY")
bindEnv(v, "plugins.trends.serp_api_key", "POWERWORD_SERP_API_KEY")

// After:
bindEnv(v, "api_keys.gemini", "POWERWORD_GEMINI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY")
bindEnv(v, "api_keys.openai", "POWERWORD_OPENAI_API_KEY", "OPENAI_API_KEY")
bindEnv(v, "api_keys.anthropic", "POWERWORD_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY")
bindEnv(v, "plugins.trends.serp_api_key", "POWERWORD_SERP_API_KEY", "SERP_API_KEY")
```

#### Update `loadDotEnv()` to expand generic var names

The current `loadDotEnv()` only processes `POWERWORD_`-prefixed keys from `.env`. Extend it to also process the canonical provider names and map them into the `POWERWORD_*` namespace when not already set:

```go
type envAlias struct {
    alias  string
    target string
}

// aliases is an ordered list of canonical provider env var aliases.
// Precedence is determined by order: aliases appearing earlier (e.g. GEMINI_API_KEY)
// take precedence over aliases appearing later (e.g. GOOGLE_API_KEY).
// When an alias is found in .env, the target POWERWORD_ var is set from the alias
// value only if neither the alias var nor the target is already set in the OS
// environment, preserving the OS-wins precedence rule.
var aliases = []envAlias{
    {alias: "GEMINI_API_KEY", target: "POWERWORD_GEMINI_API_KEY"},
    {alias: "GOOGLE_API_KEY", target: "POWERWORD_GEMINI_API_KEY"},
    {alias: "OPENAI_API_KEY", target: "POWERWORD_OPENAI_API_KEY"},
    {alias: "ANTHROPIC_API_KEY", target: "POWERWORD_ANTHROPIC_API_KEY"},
    {alias: "SERP_API_KEY", target: "POWERWORD_SERP_API_KEY"},
}
```

When a canonical key is found in `.env`, set the `POWERWORD_*` target **only if neither the canonical var nor the `POWERWORD_*` target is already set in the OS environment** (preserving the OS-wins precedence rule).

### `pkg/config/config_test.go`

Add test cases:
- `GEMINI_API_KEY` set in env → `cfg.APIKeys.Gemini` is populated.
- `POWERWORD_GEMINI_API_KEY` set alongside `GEMINI_API_KEY` → `POWERWORD_` wins.
- `GOOGLE_API_KEY` set, neither `GEMINI_API_KEY` nor `POWERWORD_GEMINI_API_KEY` set → `cfg.APIKeys.Gemini` is populated.
- `ANTHROPIC_API_KEY` → `cfg.APIKeys.Anthropic` populated.
- `OPENAI_API_KEY` → `cfg.APIKeys.OpenAI` populated.
- `SERP_API_KEY` → `cfg.Plugins.Trends.SerpAPIKey` populated.
- `.env` file with `GEMINI_API_KEY=test` → loaded and available.

---

## User-Facing Impact

After this change, the entire Borch-AI stack works with a single minimal setup:

```bash
# Shell profile or .env — provider canonical names work everywhere:
export GEMINI_API_KEY="your-google-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export SERP_API_KEY="your-serpapi-key"
```

No `POWERWORD_` prefix required. Tools that already set `POWERWORD_*` vars are unaffected — those vars continue to take highest precedence.

---

## Verification Plan

### Automated Tests
- `go test -race ./pkg/config/...` — all fallback resolution tests pass.
- `make check-coverage` — ≥91%.

### Manual Verification
```bash
# Unset POWERWORD_ vars, set only canonical names:
unset POWERWORD_GEMINI_API_KEY
export GEMINI_API_KEY="your-key"

powerword --help        # should not error "no API keys found"
kiln scout --niche "radon" --dry-run  # should fetch trend scores via pw-mcp-trends
```
