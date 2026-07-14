package amazon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
)

// AmazonService coordinates calls to the commercial Amazon Search API (ScaleSerp/Rainforest API).
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
	if apiKey == "mock" || apiKey == "" {
		// Mock fallback count for local dry-runs and tests when no key is set.
		return 4200, nil
	}

	baseURL := as.cfg.Plugins.Amazon.BaseURL
	if baseURL == "" {
		baseURL = "https://api.scaleserp.com"
	}

	u, err := url.Parse(baseURL + "/search")
	if err != nil {
		return 0, fmt.Errorf("invalid base url: %w", err)
	}

	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("q", keyword)

	// Support both ScaleSerp and Rainforest parameters:
	q.Set("search_type", "products")
	q.Set("type", "search")

	u.RawQuery = q.Encode()

	//nolint:gosec // G107: URL is constructed from pre-configured baseURL and query parameters
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := as.client.Do(req)
	if err != nil {
		errStr := err.Error()
		if apiKey != "" {
			errStr = strings.ReplaceAll(errStr, apiKey, "REDACTED")
		}
		return 0, fmt.Errorf("http request failed: %s", errStr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("api returned status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		SearchInformation struct {
			TotalResults int `json:"total_results"`
		} `json:"search_information"`
		SearchResults struct {
			TotalResults int `json:"total_results"`
		} `json:"search_results"`
		TotalResults int `json:"total_results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	// Try extracting count from multiple potential fields returned by ScaleSerp / Rainforest
	count := payload.SearchInformation.TotalResults
	if count == 0 {
		count = payload.SearchResults.TotalResults
	}
	if count == 0 {
		count = payload.TotalResults
	}

	return count, nil
}
