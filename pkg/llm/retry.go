package llm

import (
	"context"
	crand "crypto/rand"
	"errors"
	"io"
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
	MaxRetries int
	MinBackoff time.Duration
	MaxBackoff time.Duration
	Retryable  func(err error) bool
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

// sanitizeConfig ensures that all fields of RetryConfig have valid, non-zero values.
func sanitizeConfig(cfg RetryConfig) RetryConfig {
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	} else if cfg.MaxRetries == 0 && cfg.MinBackoff == 0 && cfg.MaxBackoff == 0 && cfg.Retryable == nil {
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

// Retry executes op, automatically retrying transient errors using exponential backoff with full jitter.
func Retry(ctx context.Context, cfg RetryConfig, op func() error) error {
	cfg = sanitizeConfig(cfg)

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := op()
		if err == nil {
			return nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if cfg.Retryable != nil && !cfg.Retryable(err) {
			return err
		}

		if attempt == cfg.MaxRetries {
			break
		}

		backoff := calculateBackoff(cfg.MinBackoff, cfg.MaxBackoff, attempt)
		sleep := calculateJitterSleep(backoff)

		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return lastErr
}

func calculateBackoff(minBackoff, maxBackoff time.Duration, attempt int) time.Duration {
	if attempt > 30 {
		return maxBackoff
	}
	mult := 1 << attempt
	backoff := minBackoff * time.Duration(mult)
	if backoff > maxBackoff || backoff <= 0 {
		return maxBackoff
	}
	return backoff
}

func calculateJitterSleep(backoff time.Duration) time.Duration {
	if backoff <= 0 {
		return 0
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(backoff)+1))
	if err != nil {
		return backoff
	}
	return time.Duration(n.Int64())
}

// IsRetryableError identifies whether an error is transient and should be retried.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Never retry explicit context cancellation
	if errors.Is(err, context.Canceled) {
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

	// OpenAI Request Error
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return isRetryableStatusCode(reqErr.HTTPStatusCode)
	}

	// Anthropic API Error
	var aErr *anthropic.Error
	if errors.As(err, &aErr) {
		return isRetryableStatusCode(aErr.StatusCode)
	}

	// Network temporary or timeout errors
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Connection resets or premature EOFs
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	// Fallback to error message inspection
	return matchesTransientString(err.Error())
}

func isRetryableStatusCode(code int) bool {
	switch code {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

var (
	statusCodePattern   = regexp.MustCompile(`(?i)\b(?:status(?:\s*code)?|http|code|error|transient)\s*[:=]?\s*(408|429|500|502|503|504)\b`)
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
