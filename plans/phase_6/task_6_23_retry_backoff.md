# plan: Task 6.23: Standardized Request Retry Engine with Exponential Backoff

**Status:** Open
**Go Version:** 1.26.4
**Date Completed:**
**Unit Test Coverage:**

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

- Wrap call-out actions (`Generate` and `Stream`) inside the retry helper.
- Standardize error mapping to identify transient HTTP status codes (429, 502, 503, 504) as retryable.

### `internal/plugins/imagegen/`

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

- Wrap `GoogleBackend.GenerateImage`, `VeoBackend.GenerateImage`, `MidjourneyBackend.GenerateImage`, and the `downloadImage` functions with the retry helper.

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
  ```

- Verify code coverage:

  ```bash
  make check-coverage
  ```
