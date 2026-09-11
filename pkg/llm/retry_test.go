package llm

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/googleapi"
)

type mockNetTimeoutError struct{}

func (e *mockNetTimeoutError) Error() string   { return "i/o timeout" }
func (e *mockNetTimeoutError) Timeout() bool   { return true }
func (e *mockNetTimeoutError) Temporary() bool { return true }

func TestRetry_SuccessImmediate(t *testing.T) {
	ctx := context.Background()
	var calls int
	cfg := RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	}

	err := Retry(ctx, cfg, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call, got: %d", calls)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	var calls int
	cfg := RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
		Retryable: func(err error) bool {
			return true
		},
	}

	err := Retry(ctx, cfg, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient failure 503")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got: %d", calls)
	}
}

func TestRetry_ExhaustRetries(t *testing.T) {
	ctx := context.Background()
	var calls int
	cfg := RetryConfig{
		MaxRetries: 2,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 5 * time.Millisecond,
		Retryable: func(err error) bool {
			return true
		},
	}

	expectedErr := errors.New("persistent error")
	err := Retry(ctx, cfg, func() error {
		calls++
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got: %v", expectedErr, err)
	}
	if calls != 3 { // initial attempt + 2 retries = 3
		t.Fatalf("expected 3 total attempts, got: %d", calls)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	var calls int
	cfg := RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
		Retryable: func(err error) bool {
			return strings.HasPrefix(err.Error(), "transient-")
		},
	}

	fatalErr := errors.New("fatal non-retryable 400 error")
	err := Retry(ctx, cfg, func() error {
		calls++
		return fatalErr
	})

	if !errors.Is(err, fatalErr) {
		t.Fatalf("expected %v, got: %v", fatalErr, err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call for non-retryable error, got: %d", calls)
	}
}

func TestRetry_ContextCanceledBeforeExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	cfg := DefaultRetryConfig()
	err := Retry(ctx, cfg, func() error {
		calls++
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 calls, got: %d", calls)
	}
}

func TestRetry_ContextCanceledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls int
	cfg := RetryConfig{
		MaxRetries: 3,
		MinBackoff: 400 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
		Retryable:  func(err error) bool { return true },
	}

	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := Retry(ctx, cfg, func() error {
		calls++
		return errors.New("transient error")
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("expected fast cancellation, took %v", elapsed)
	}
	if calls != 1 {
		t.Errorf("expected 1 call before cancellation during backoff, got %d", calls)
	}
}

func TestRetry_ContextCanceledDuringOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cfg := DefaultRetryConfig()
	err := Retry(ctx, cfg, func() error {
		cancel()
		return errors.New("error during cancel")
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestRetry_DefaultConfigSanitization(t *testing.T) {
	// Passing completely uninitialized RetryConfig{}
	var calls int32
	err := Retry(context.Background(), RetryConfig{}, func() error {
		if atomic.AddInt32(&calls, 1) < 2 {
			return errors.New("transient 429")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got: %d", calls)
	}

	// Negative MaxRetries should be sanitized to 0
	calls = 0
	err = Retry(context.Background(), RetryConfig{MaxRetries: -1}, func() error {
		atomic.AddInt32(&calls, 1)
		return errors.New("transient 500")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call for MaxRetries: -1, got: %d", calls)
	}
}

func TestCalculateBackoff(t *testing.T) {
	minB := 100 * time.Millisecond
	maxB := 1 * time.Second

	// attempt 0
	b0 := calculateBackoff(minB, maxB, 0)
	if b0 != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", b0)
	}

	// attempt 1
	b1 := calculateBackoff(minB, maxB, 1)
	if b1 != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", b1)
	}

	// attempt 2
	b2 := calculateBackoff(minB, maxB, 2)
	if b2 != 400*time.Millisecond {
		t.Errorf("expected 400ms, got %v", b2)
	}

	// attempt 5 (would be 3200ms, should clamp to maxB)
	b5 := calculateBackoff(minB, maxB, 5)
	if b5 != maxB {
		t.Errorf("expected %v, got %v", maxB, b5)
	}

	// attempt > 30
	b35 := calculateBackoff(minB, maxB, 35)
	if b35 != maxB {
		t.Errorf("expected %v, got %v", maxB, b35)
	}
}

func TestCalculateJitterSleep(t *testing.T) {
	if sleep := calculateJitterSleep(0); sleep != 0 {
		t.Errorf("expected 0 for backoff 0, got %v", sleep)
	}
	if sleep := calculateJitterSleep(-10 * time.Millisecond); sleep != 0 {
		t.Errorf("expected 0 for negative backoff, got %v", sleep)
	}

	backoff := 100 * time.Millisecond
	var hasUnderHalf bool
	for i := 0; i < 100; i++ {
		sleep := calculateJitterSleep(backoff)
		if sleep < 0 || sleep > backoff {
			t.Fatalf("jitter sleep %v out of range [0, %v]", sleep, backoff)
		}
		if sleep < 50*time.Millisecond {
			hasUnderHalf = true
		}
	}
	if !hasUnderHalf {
		t.Errorf("expected full jitter to sample across the entire [0, backoff] range including < 50ms")
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"context.Canceled", context.Canceled, false},
		{"googleapi 429", &googleapi.Error{Code: 429}, true},
		{"googleapi 500", &googleapi.Error{Code: 500}, true},
		{"googleapi 502", &googleapi.Error{Code: 502}, true},
		{"googleapi 503", &googleapi.Error{Code: 503}, true},
		{"googleapi 504", &googleapi.Error{Code: 504}, true},
		{"googleapi 400", &googleapi.Error{Code: 400}, false},
		{"googleapi 404", &googleapi.Error{Code: 404}, false},
		{"openai APIError 429", &openai.APIError{HTTPStatusCode: 429}, true},
		{"openai APIError 500", &openai.APIError{HTTPStatusCode: 500}, true},
		{"openai APIError 502", &openai.APIError{HTTPStatusCode: 502}, true},
		{"openai APIError 503", &openai.APIError{HTTPStatusCode: 503}, true},
		{"openai APIError 400", &openai.APIError{HTTPStatusCode: 400}, false},
		{"openai APIError 404", &openai.APIError{HTTPStatusCode: 404}, false},
		{"openai RequestError 429", &openai.RequestError{HTTPStatusCode: 429}, true},
		{"openai RequestError 503", &openai.RequestError{HTTPStatusCode: 503}, true},
		{"openai RequestError 400", &openai.RequestError{HTTPStatusCode: 400}, false},
		{"anthropic Error 429", &anthropic.Error{StatusCode: 429}, true},
		{"anthropic Error 500", &anthropic.Error{StatusCode: 500}, true},
		{"anthropic Error 502", &anthropic.Error{StatusCode: 502}, true},
		{"anthropic Error 503", &anthropic.Error{StatusCode: 503}, true},
		{"anthropic Error 400", &anthropic.Error{StatusCode: 400}, false},
		{"anthropic Error 404", &anthropic.Error{StatusCode: 404}, false},
		{"net.Error timeout", &mockNetTimeoutError{}, true},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"string pattern 429", errors.New("HTTP 429: Too Many Requests"), true},
		{"string pattern resource exhausted", errors.New("rpc error: code = ResourceExhausted desc = Quota exceeded"), true},
		{"string pattern 502 bad gateway", errors.New("502 Bad Gateway"), true},
		{"string pattern 503 service unavailable", errors.New("503 Service Unavailable"), true},
		{"string pattern connection reset", errors.New("read: connection reset by peer"), true},
		{"string pattern broken pipe", errors.New("write: broken pipe"), true},
		{"non-HTTP bare numbers not retryable", errors.New("validation error mentioning a 500-token limit"), false},
		{"dimension 429 not retryable", errors.New("image dimension 429 is not supported"), false},
		{"unrelated error", errors.New("file not found: config.yaml"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsRetryableError(tc.err)
			if got != tc.expected {
				t.Errorf("IsRetryableError(%v) = %v; expected %v", tc.err, got, tc.expected)
			}
		})
	}
}

func TestMatchesTransientString(t *testing.T) {
	transientMessages := []string{
		"rate limit exceeded",
		"504 gateway timeout",
		"connection refused by server",
		"unexpected eof reading response",
		"tls: handshake timeout",
		"server disconnected suddenly",
		"HTTP 429 Too Many Requests",
		"status code: 500",
		"status 502",
		"rpc error: code = 503 desc = service unavailable",
		"server overloaded",
		"transient 429 error",
	}

	for _, msg := range transientMessages {
		if !matchesTransientString(msg) {
			t.Errorf("expected %q to be identified as transient string", msg)
		}
	}

	for _, msg := range []string{
		"unauthorized invalid token",
		"validation error mentioning a 500-token limit",
		"invalid dimension 429",
		"file size 502 bytes exceeds limit",
		"user id 504 not found",
	} {
		if matchesTransientString(msg) {
			t.Errorf("did not expect %q to be identified as transient", msg)
		}
	}
}

func TestIsRetryableStatusCode(t *testing.T) {
	for _, code := range []int{408, 429, 500, 502, 503, 504} {
		if !isRetryableStatusCode(code) {
			t.Errorf("expected code %d to be retryable", code)
		}
	}

	for _, code := range []int{200, 201, 400, 401, 403, 404, 409, 422} {
		if isRetryableStatusCode(code) {
			t.Errorf("expected code %d to not be retryable", code)
		}
	}
}
