# plan: Task 6.22: Google Backend Image/Video Generation Timeout Adjustment

**Status:** Pending
**Go Version:** 1.26.4
**Date Completed:** 
**Unit Test Coverage:** 

---

## User Review Required

> [!NOTE]
> No breaking changes. Increases the default HTTP client timeout from 30 seconds to 120 seconds in `pw-mcp-imagegen` backends, ensuring synchronous Imagen REST requests have enough time to complete under load.

---

## Problem

When using `pithos brew`, generation of the character seed portrait or page illustrations using Google's Imagen model (`imagen-4.0-generate-001` or `imagen-3.0-generate-002`) can fail with a timeout:

```
failed to generate character seed portrait: tool execution failed: failed to generate image: google predict request failed: Post "https://generativelanguage.googleapis.com/v1beta/models/imagen-4.0-generate-001:predict?key=...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

In `pw-mcp-imagegen`, Google's Imagen API is invoked synchronously via the Google AI Studio predict REST endpoint (`:predict`), which blocks until the image is fully generated. The `GoogleBackend` uses an `http.Client` initialized with a hardcoded timeout of `30 * time.Second`. Under ordinary or high model load, image generation can easily exceed 30 seconds, causing the HTTP client context to time out.

Similarly, the `VeoBackend` (for video generation) and `MidjourneyBackend` (for Midjourney API requests) clients are also initialized with a hardcoded `30 * time.Second` HTTP client timeout.

---

## Goal

Increase the hardcoded HTTP client timeout in `GoogleBackend`, `VeoBackend`, and `MidjourneyBackend` from `30 * time.Second` to `120 * time.Second` to allow sufficient time for synchronous generation requests and image downloads to complete successfully.

---

## Proposed Changes

### `internal/plugins/imagegen/imagegen.go`

#### [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

1. In `NewGoogleBackend` (around line 177), increase the `Timeout` field of the `http.Client`:
   ```diff
    	return &GoogleBackend{
    		apiURL: apiURL,
    		apiKey: apiKey,
    		model:  model,
   -		client: &http.Client{Timeout: 30 * time.Second},
   +		client: &http.Client{Timeout: 120 * time.Second},
    	}
   ```

2. In `NewVeoBackend` (around line 304), increase the `Timeout` field of the `http.Client`:
   ```diff
    	return &VeoBackend{
    		apiURL:          apiURL,
    		apiKey:          apiKey,
    		model:           model,
    		pollingInterval: interval,
    		pollingTimeout:  timeout,
   -		client:          &http.Client{Timeout: 30 * time.Second},
   +		client:          &http.Client{Timeout: 120 * time.Second},
    	}, nil
   ```

3. In `NewMidjourneyBackend` (around line 543), increase the `Timeout` field of the `httpClient`:
   ```diff
    	return &MidjourneyBackend{
    		apiURL:          apiURL,
    		apiKey:          apiKey,
    		pollingInterval: interval,
    		pollingTimeout:  timeout,
   -		httpClient:      &http.Client{Timeout: 30 * time.Second},
   +		httpClient:      &http.Client{Timeout: 120 * time.Second},
    	}, nil
   ```

---

## Verification Plan

### Automated Tests
- Run the `imagegen` plugin unit tests to ensure no regressions:
  ```bash
  go test -v ./internal/plugins/imagegen/...
  ```
- Ensure the project-wide 91% unit test coverage threshold is maintained by running:
  ```bash
  make check-coverage
  ```

### Manual Verification
1. Re-compile the `powerword` binaries using `make build`.
2. Verify the `pw-mcp-imagegen` binary in the target location is updated.
3. Run `pithos brew --output books/frog_book` with the Imagen model enabled to ensure it generates the character seed portrait successfully without encountering a 30-second timeout.
