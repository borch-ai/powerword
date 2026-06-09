# Task 4.3: Telemetry, Token Metrics & Cost Accounting

Implement tracking and reporting of LLM token metrics (input tokens, output tokens, cached/context tokens) across execution loop turns. Calculate and render estimated cost calculations on CLI termination.

## Status: Completed

## Final Implementation Details

- **Configuration (`config.go`)**: Added `Pricing map[string]ModelPricing` to map model names (and prefixes) to `Input`, `Output`, and `Cached` costs per 1M tokens.
- **Telemetry (`telemetry.go`)**: Added `UsageTracker` to aggregate tokens per model. Added `EstimatedCost` with prefix-matching logic for models.
- **Provider SDKs (`gemini.go`, `openai.go`, `anthropic.go`)**: Hooked into both Generate and Stream APIs for all three supported LLM providers to extract and pass back token counts. Resolved streaming challenges by leveraging native SDK stream options and message events instead of client-side tokenizers.
- **Execution Loop (`loop.go`)**: Instantiates `UsageTracker` per session, captures tokens per turn, and prints formatted summary panel at session termination. Added usage metadata to `JSONPayload` in headless mode.
- **Testing**: Added `telemetry_test.go` to test model prefix matching and math. Passed `make check-coverage` with `91.0%` test coverage.
