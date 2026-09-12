package llm

import (
	"context"
	crand "crypto/rand"
	"errors"
	"io"
	"math"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/googleapi"
)

// RetryConfig defines the parameters for the request retry engine.
type RetryConfig struct {
	Disabled   bool
	MaxRetries int
	MinBackoff time.Duration
	MaxBackoff time.Duration
	Retryable  func(err error) bool
	Sleep      func(ctx context.Context, d time.Duration) error
}

// DefaultRetryConfig returns a standard retry configuration suitable for GenAI requests.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		MinBackoff: 100 * time.Millisecond,
		MaxBackoff: 5 * time.Second,
		Retryable:  IsRetryableError,
	}
}

// NoRetries returns a RetryConfig that disables retries, executing operations at most once.
func NoRetries() RetryConfig {
	return RetryConfig{
		Disabled:   true,
		MaxRetries: 0,
	}
}

// sanitizeConfig ensures that all fields of RetryConfig have valid values.
func sanitizeConfig(cfg RetryConfig) RetryConfig {
	if cfg.Disabled || cfg.MaxRetries < 0 {
		cfg.Disabled = true
		cfg.MaxRetries = 0
		return cfg
	}

	if cfg.MaxRetries == 0 && cfg.MinBackoff == 0 && cfg.MaxBackoff == 0 && cfg.Retryable == nil && cfg.Sleep == nil {
		return DefaultRetryConfig()
	}

	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff < cfg.MinBackoff {
		cfg.MaxBackoff = cfg.MinBackoff
	}
	if cfg.Retryable == nil {
		cfg.Retryable = IsRetryableError
	}
	return cfg
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Retry executes op, automatically retrying transient errors using exponential backoff with full jitter.
// If op returns an error that the configured Retryable classifier (defaulting to IsRetryableError) classifies as non-transient,
// or if retries are exhausted, Retry returns the last encountered error. If ctx is canceled during execution or backoff, ctx.Err() is returned.
func Retry(ctx context.Context, cfg RetryConfig, op func() error) error {
	cfg = sanitizeConfig(cfg)

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		err := op()
		if err == nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return nil
		}
		lastErr = err

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		// Fail fast if not retryable or on the final attempt
		if cfg.Disabled || attempt == cfg.MaxRetries || !cfg.Retryable(err) {
			return lastErr
		}

		backoff := calculateBackoff(cfg.MinBackoff, cfg.MaxBackoff, attempt)
		sleepDuration := calculateJitterSleep(backoff)

		sleeper := cfg.Sleep
		if sleeper == nil {
			sleeper = defaultSleep
		}
		if sleepErr := sleeper(ctx, sleepDuration); sleepErr != nil {
			return sleepErr
		}
	}

	return lastErr
}

func calculateBackoff(minBackoff, maxBackoff time.Duration, attempt int) time.Duration {
	if attempt > 30 || minBackoff <= 0 {
		return maxBackoff
	}
	mult := time.Duration(1 << attempt)
	if maxBackoff/mult < minBackoff {
		return maxBackoff
	}
	backoff := minBackoff * mult
	if backoff > maxBackoff || backoff <= 0 {
		return maxBackoff
	}
	return backoff
}

func calculateJitterSleep(backoff time.Duration) time.Duration {
	if backoff <= 0 {
		return 0
	}
	maxBound := new(big.Int).SetUint64(uint64(backoff) + 1)
	n, err := crand.Int(crand.Reader, maxBound)
	if err != nil {
		return backoff
	}
	if !n.IsInt64() {
		return math.MaxInt64
	}
	return time.Duration(n.Int64())
}

// IsRetryableError identifies whether an error is transient and should be retried.
func IsRetryableError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Google API Error
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		return isRetryableStatusCode(gErr.Code)
	}

	// OpenAI API Error
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return isRetryableStatusCode(apiErr.HTTPStatusCode)
	}

	// OpenAI Request Error (only match if HTTPStatusCode is nonzero, otherwise fall through to transport checks)
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) && reqErr.HTTPStatusCode != 0 {
		return isRetryableStatusCode(reqErr.HTTPStatusCode)
	}

	// Anthropic API Error
	var aErr *anthropic.Error
	if errors.As(err, &aErr) {
		return isRetryableStatusCode(aErr.StatusCode)
	}

	// Network / Timeout Errors
	var netErr net.Error
	//nolint:staticcheck // netErr.Temporary() provides fallback for temporary network errors
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	return matchesTransientString(err.Error())
}

func isRetryableStatusCode(code int) bool {
	switch code {
	case 408, 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
	}
}

var (
	statusCodePattern   = regexp.MustCompile(`(?i)\b(?:status(?:\s*code)?|http|code|error|transient)\s*[:=]?\s*(408|429|500|502|503|504|529)(?:$|[\s:;,\.\]\)])`)
	statusPhrasePattern = regexp.MustCompile(`(?i)\b(?:408\s+request\s+timeout|429\s+too\s+many\s+requests|500\s+internal\s+server\s+error|502\s+bad\s+gateway|503\s+service\s+unavailable|504\s+gateway\s+timeout)\b`)
)

func matchesTransientString(msg string) bool {
	if statusCodePattern.MatchString(msg) || statusPhrasePattern.MatchString(msg) {
		return true
	}

	lower := strings.ToLower(msg)
	patterns := []string{
		"too many requests",
		"resource_exhausted",
		"resourceexhausted",
		"quota exceeded",
		"quota_exceeded",
		"rate limit",
		"ratelimit",
		"internal server error",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected eof",
		"tls: handshake",
		"server disconnected",
		"overloaded",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
