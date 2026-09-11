# plan: Task 6.23: Standardized Request Retry Engine with Exponential Backoff

**Status:** Complete
**Go Version:** 1.26.6
**Date Completed:** 2026-09-11
**Unit Test Coverage:** 91.10%

---

## User Review Required

> [!NOTE]
> No breaking changes. This plan introduces a retry policy helper inside the codebase to automatically retry transient GenAI API rate-limits and server errors.

---

## Problem

Under heavy usage or network congestion, model providers and image generation services can return transient errors (e.g. `429 Too Many Requests`, `502 Bad Gateway`, `503 Service Unavailable`, or connection resets). Currently, Powerword does not attempt any retry, causing the entire autonomous agent task loop to immediately abort and fail.

---

## Goal

Design and implement a reusable retry utility implementing exponential backoff with random jitter. Integrate this retry engine into:

1. Sibling provider client wrappers in `pkg/llm` (Gemini, OpenAI, Anthropic adapters).
2. The image/video generators and downloaders in `internal/plugins/imagegen`.

---

## Proposed Changes

### [NEW] [retry.go](file://../../pkg/llm/retry.go)

Create a lightweight request retry helper in the `llm` package:

- Implement a `Retry` helper function accepting dynamic configuration:

  ```go
  type RetryConfig struct {
      MaxRetries  int
      MinBackoff  time.Duration
      MaxBackoff  time.Duration
      Retryable   func(err error) bool
  }
  ```

- Use standard exponential backoff with full jitter to avoid synchronous hammering of API backends.

### `pkg/llm/` Provider Adapters

#### [MODIFY] [gemini.go](file://../../pkg/llm/gemini.go)

#### [MODIFY] [openai.go](file://../../pkg/llm/openai.go)

#### [MODIFY] [anthropic.go](file://../../pkg/llm/anthropic.go)

- Wrap call-out actions (`Generate` and `Stream`) inside the retry helper, preserving the synchronous setup contract for `Stream()`.
- Handle clean empty streams (`io.EOF` on initial chunk) as successful completions rather than retryable transport errors.
- Standardize error mapping to identify transient HTTP status codes (408, 429, 500, 502, 503, 504, 529) as retryable.

### `internal/plugins/imagegen/`

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

- Default backend constructors (`OpenAIBackend`, `GoogleBackend`, `VeoBackend`, `MidjourneyBackend`) to `llm.NoRetries()` (`Disabled: true`) to ensure non-idempotent image/video generation POST operations are strictly opt-in via `SetRetryConfig`, preventing duplicate jobs or billing on network resets.
- Expose `RetryConfigFromConfig(cfg config.ImageGenConfig) llm.RetryConfig` helper to share retry configuration logic across plugins.
- Retain retry capabilities on idempotent GET downloads (`downloadImage`, `downloadVideo`) and polling operations, streaming image downloads directly to temporary files with atomic commit and robust error cleanup, while bounding video downloads in memory to 100MB via `io.LimitReader`.

### `internal/plugins/viral/`

#### [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)

- Wire `ViralService` to propagate `ImageGenConfig` retry settings to constructed `VeoBackend` instances and provide `SetRetryConfig`.

---

## Verification Plan

### Automated Tests

- Create unit tests for `Retry` helper in `pkg/llm/retry_test.go` with mock transient failures and ensure:
  - It retries the configured number of times.
  - It respects backoff intervals.
  - It exits immediately on non-retryable errors.
- Run tests:

  ```bash
  go test -v ./pkg/llm/...
  go test -v ./internal/plugins/imagegen/...
  go test -v ./internal/plugins/viral/...
  ```

- Verify code coverage:

  ```bash
  make check-coverage
  ```

  Verified passing with 91.10% coverage (exceeding the >= 91.00% requirement on Go 1.26).
