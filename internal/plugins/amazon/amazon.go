package amazon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
)

// AmazonService coordinates calls to the commercial ScaleSerp Amazon Search API.
type AmazonService struct {
	cfg    *config.Config
	client *http.Client
}

// NewAmazonService constructs a new AmazonService.
func NewAmazonService(cfg *config.Config, client *http.Client) *AmazonService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &AmazonService{
		cfg:    cfg,
		client: client,
	}
}

// GetListingCount queries the commercial API for the search result count.
func (as *AmazonService) GetListingCount(ctx context.Context, keyword string) (int, error) {
	apiKey := as.cfg.Plugins.Amazon.APIKey
	if apiKey == "mock" {
		// Mock fallback count for local dry-runs and tests when explicit mock is set.
		return 4200, nil
	}
	if apiKey == "" {
		return 0, errors.New("amazon api key is required (set to 'mock' for local offline testing)")
	}

	u, err := as.buildURL(apiKey, keyword)
	if err != nil {
		return 0, err
	}

	//nolint:gosec // G107: URL is constructed from pre-configured baseURL and query parameters
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := as.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http request failed: %w", &redactedError{err: err, apiKey: apiKey})
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("api returned status %d: %s", resp.StatusCode, redactKey(string(bodyBytes), apiKey))
	}

	return parseResponsePayload(resp.Body)
}

type redactedError struct {
	err    error
	apiKey string
}

func (e *redactedError) Error() string {
	return redactKey(e.err.Error(), e.apiKey)
}

func (e *redactedError) Unwrap() error {
	return e.err
}

func redactKey(str, apiKey string) string {
	if apiKey == "" {
		return str
	}
	str = strings.ReplaceAll(str, apiKey, "REDACTED")
	encoded := url.QueryEscape(apiKey)
	if encoded != apiKey {
		str = strings.ReplaceAll(str, encoded, "REDACTED")
	}
	return str
}

func (as *AmazonService) buildURL(apiKey, keyword string) (*url.URL, error) {
	baseURL := as.cfg.Plugins.Amazon.BaseURL
	if baseURL == "" {
		baseURL = "https://api.scaleserp.com"
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	if parsedBase.Scheme != "http" && parsedBase.Scheme != "https" {
		return nil, fmt.Errorf("invalid base url scheme %q: must be http or https", parsedBase.Scheme)
	}
	if parsedBase.Host == "" {
		return nil, fmt.Errorf("invalid base url: missing host")
	}

	if parsedBase.Scheme == "http" {
		hn := parsedBase.Hostname()
		if hn != "localhost" && hn != "127.0.0.1" && hn != "::1" {
			return nil, fmt.Errorf("insecure http scheme is only allowed for loopback hosts (localhost, 127.0.0.1, ::1); got %q", hn)
		}
	}

	u := parsedBase.JoinPath("search")

	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("q", keyword)

	// Set query parameters for ScaleSerp Search:
	q.Set("search_type", "products")

	u.RawQuery = q.Encode()
	return u, nil
}

func parseResponsePayload(body io.Reader) (int, error) {
	var payload struct {
		SearchInformation struct {
			TotalResults int `json:"total_results"`
		} `json:"search_information"`
		SearchResults json.RawMessage `json:"search_results"`
		TotalResults  int             `json:"total_results"`
	}

	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	// Try extracting count from multiple potential fields returned by ScaleSerp
	count := payload.SearchInformation.TotalResults
	if count == 0 {
		count = payload.TotalResults
	}
	trimmed := bytes.TrimSpace(payload.SearchResults)
	if count == 0 && len(trimmed) > 0 && trimmed[0] == '{' {
		var searchResultsObj struct {
			TotalResults int `json:"total_results"`
		}
		if err := json.Unmarshal(trimmed, &searchResultsObj); err != nil {
			return 0, fmt.Errorf("failed to decode search_results object: %w", err)
		}
		count = searchResultsObj.TotalResults
	}

	return count, nil
}
