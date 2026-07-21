# plan: Task 6.22: Configurable & Generalized Image/Video Generation Timeouts

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-18
**Unit Test Coverage:** 91.0%

---

## User Review Required

> [!NOTE]
> No breaking changes. Introduces a new configuration parameter `plugins.imagegen.request_timeout`, defaulting to `120s`, to specify the maximum duration allowed for HTTP requests made during image/video generation and image downloading.

---

## Problem

When using `pithos brew`, generation of the character seed portrait or page illustrations using Google's Imagen model (`imagen-4.0-generate-001` or `imagen-3.0-generate-002`) can fail with a timeout:

```text
failed to generate character seed portrait: tool execution failed: failed to generate image: google predict request failed: Post "https://generativelanguage.googleapis.com/v1beta/models/imagen-4.0-generate-001:predict?key=...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

In `pw-mcp-imagegen`, Google's Imagen API is invoked synchronously via the Google AI Studio predict REST endpoint (`:predict`), which blocks until the image is fully generated. The `GoogleBackend` uses an `http.Client` initialized with a hardcoded timeout of `30 * time.Second`. Under ordinary or high model load, image generation can easily exceed 30 seconds, causing the HTTP client context to time out.

Similarly, the `VeoBackend` (for video generation) and `MidjourneyBackend` (for Midjourney API requests) clients are also initialized with a hardcoded `30 * time.Second` HTTP client timeout.

---

## Goal

Generalize image and video generation timeouts by making them configurable via a new configuration parameter `plugins.imagegen.request_timeout` (and corresponding environment variable `POWERWORD_IMAGEGEN_REQUEST_TIMEOUT`), defaulting to `120 * time.Second` across all backends (Google Imagen, Google Veo, Midjourney, OpenAI DALL-E) and download helpers.

---

## Proposed Changes

### Configuration Management

#### [MODIFY] [config.go](file://../../pkg/config/config.go)

1. Add `RequestTimeout` string field to `ImageGenConfig` struct.
2. Set default value in `LoadConfig` (`plugins.imagegen.request_timeout` -> `120s`).
3. Bind the environment variable `POWERWORD_IMAGEGEN_REQUEST_TIMEOUT`.

### ImageGen Plugin

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

1. Add `SetTimeout(time.Duration)` exported methods to `GoogleBackend`, `VeoBackend`, and `MidjourneyBackend` to allow timeout overrides without breaking constructor signatures.
2. Update default timeouts inside existing constructors from `30 * time.Second` to `120 * time.Second`.
3. Add `NewOpenAIBackendWithTimeout` helper and initialize client with 120s default timeout.
4. Define `getRequestTimeout()` helper on `ImageGenService` to parse configured timeout.
5. In `runGoogle`, `runVeo`, `runMidjourney`, and `runOpenAI` methods, apply the custom timeout override.
6. Update `downloadImage` function to accept `timeout time.Duration` parameter and initialize a custom `http.Client` with it.

### Viral Plugin

#### [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)

1. In `runVeo`, retrieve the configured timeout and set it on the created backend before generation.

---

## Verification Plan

### Automated Tests

- Run unit tests for `imagegen` and `viral` packages:

  ```bash
  go test -v ./internal/plugins/imagegen/...
  go test -v ./internal/plugins/viral/...
  ```

- Run the full project-wide checks to verify 91% code coverage:

  ```bash
  make check-coverage
  ```

### Manual Verification

- Compile powerword: `make build`
- Verify image generation executes successfully using various backends.
