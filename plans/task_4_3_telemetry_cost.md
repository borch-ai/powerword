# Task 4.3: Telemetry, Token Metrics & Cost Accounting

Implement tracking and reporting of LLM token metrics (input tokens, output tokens, cached/context tokens) across execution loop turns. Calculate and render estimated cost calculations on CLI termination.

## User Review Required

> [!NOTE]
> Pricing Configuration: Model prices vary frequently. We will load pricing coefficients ($ per 1M tokens) from the Viper config file (`config.yaml`), fallback to standard static coefficients if absent.

## Proposed Changes

### Token Trackers & Reporting Engine

#### [NEW] [telemetry.go](file:///Users/human/code/powerword/internal/llm/telemetry.go)
- Defines structures:
  - `UsageTracker` recording token tallies, runtimes, and loop count.
  - `ModelPricing` mapping pricing configurations per 1M tokens.
- Implements:
  - `RecordUsage(model string, input, output, cached int)`
  - `FormatSummary() string`

#### [MODIFY] [loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Captures API usage parameters returned by LLM response payloads (Gemini's `UsageMetadata` or OpenAI's `Usage` fields).
- Aggregates usage data into `UsageTracker`.
- On exit, prints a detailed session recap (color-coded, showing exact input/output tokens, cached token reuse, and estimated pricing in USD).

---

## Verification Plan

### Automated Tests
- Test aggregation logic in `UsageTracker` to ensure counts across multiple turns are summed correctly.
- Verify price calculation logic under different models, ensuring float precision does not lead to rounding glitches.

### Manual Verification
- Execute any query and confirm that on termination, the console prints a clean panel detailing:
  ```
  Session Metrics:
  - Total Tokens: 4,500 (3,000 In, 1,500 Out)
  - Estimated Cost: $0.034
  ```
