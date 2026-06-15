package trends

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
)

// Candidate represents a keyword candidate with demand and velocity signals.
type Candidate struct {
	Keyword     string  `json:"keyword"`
	Completions int     `json:"completions"` // Amazon completion rank (0 if source is serp-only)
	TrendScore  float64 `json:"trend_score"` // Cap at 1.0
}

// TrendSource is the extension interface for market intelligence backends.
type TrendSource interface {
	Score(ctx context.Context, keyword string, limit int) ([]Candidate, error)
	Name() string
}

// AmazonAutocomplete fetches completions from Amazon Autocomplete API.
type AmazonAutocomplete struct {
	client *http.Client
}

// NewAmazonAutocomplete creates a new AmazonAutocomplete source.
func NewAmazonAutocomplete(client *http.Client) *AmazonAutocomplete {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &AmazonAutocomplete{client: client}
}

// Name returns the source name.
func (a *AmazonAutocomplete) Name() string {
	return "amazon"
}

// Score queries the Amazon autocomplete suggestions.
func (a *AmazonAutocomplete) Score(ctx context.Context, keyword string, limit int) ([]Candidate, error) {
	escapedQuery := url.QueryEscape(keyword)
	baseURL := "https://completion.amazon.com"
	if envURL := os.Getenv("POWERWORD_AMAZON_BASE_URL"); envURL != "" {
		baseURL = envURL
	}
	u := fmt.Sprintf("%s/search/complete?search-alias=stripbooks&client=amazon-search-ui&mkt=1&q=%s", baseURL, escapedQuery)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Amazon request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("amazon request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("amazon API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Amazon response: %w", err)
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse Amazon autocomplete response: %w", err)
	}

	if len(raw) <= 1 {
		return []Candidate{}, nil
	}

	var suggestions []string
	if err := json.Unmarshal(raw[1], &suggestions); err != nil {
		return nil, fmt.Errorf("failed to parse suggestions list: %w", err)
	}

	if len(suggestions) == 0 {
		return []Candidate{}, nil
	}

	if limit > 0 && len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	candidates := make([]Candidate, len(suggestions))
	for i, suggestion := range suggestions {
		candidates[i] = Candidate{
			Keyword:     suggestion,
			Completions: len(suggestions) - i,
			TrendScore:  0.0,
		}
	}

	return candidates, nil
}

// SerpAPITrends fetches trends data from SerpAPI.
type SerpAPITrends struct {
	client *http.Client
	apiKey string
}

// NewSerpAPITrends creates a new SerpAPITrends source.
func NewSerpAPITrends(client *http.Client, apiKey string) *SerpAPITrends {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &SerpAPITrends{
		client: client,
		apiKey: apiKey,
	}
}

// Name returns the source name.
func (s *SerpAPITrends) Name() string {
	return "serp"
}

// Score implements TrendSource, querying trends for a single keyword.
func (s *SerpAPITrends) Score(ctx context.Context, keyword string, limit int) ([]Candidate, error) {
	if s.apiKey == "" {
		return []Candidate{}, nil
	}
	score, _, err := s.GetVelocity(ctx, keyword, "today 3-m")
	if err != nil {
		return nil, err
	}
	return []Candidate{
		{
			Keyword:     keyword,
			Completions: 0,
			TrendScore:  score,
		},
	}, nil
}

// TimelineValue represents a value structure in SerpAPI timeline entry.
type TimelineValue struct {
	Query          string      `json:"query"`
	Value          string      `json:"value"`
	ExtractedValue interface{} `json:"extracted_value"` // Can be float64 or string
}

// TimelineEntry represents a single entry in SerpAPI Google Trends interest_over_time.
type TimelineEntry struct {
	Date      string          `json:"date"`
	Timestamp string          `json:"timestamp"`
	Values    []TimelineValue `json:"values"`
}

// SerpAPITrendsResponse represents the parsed structure from SerpAPI.
type SerpAPITrendsResponse struct {
	InterestOverTime struct {
		TimelineData []TimelineEntry `json:"timeline_data"`
	} `json:"interest_over_time"`
}

// fetchSerpAPIData handles the HTTP request and decoding for Google Trends.
func (s *SerpAPITrends) fetchSerpAPIData(ctx context.Context, keyword string, period string) (*SerpAPITrendsResponse, error) {
	dateParam := period
	if dateParam == "" {
		dateParam = "today 3-m"
	} else if !strings.HasPrefix(dateParam, "today ") && !strings.HasPrefix(dateParam, "now ") && dateParam != "all" {
		dateParam = "today " + dateParam
	}

	escapedQuery := url.QueryEscape(keyword)
	baseURL := "https://serpapi.com"
	if envURL := os.Getenv("POWERWORD_SERPAPI_BASE_URL"); envURL != "" {
		baseURL = envURL
	}
	u := fmt.Sprintf("%s/search?engine=google_trends&q=%s&api_key=%s&data_type=TIMESERIES&date=%s", baseURL, escapedQuery, s.apiKey, url.QueryEscape(dateParam))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create SerpAPI request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SerpAPI request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SerpAPI returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var data SerpAPITrendsResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&data); decodeErr != nil {
		return nil, fmt.Errorf("failed to decode SerpAPI response: %w", decodeErr)
	}

	return &data, nil
}

// getVal extracts a float64 from ExtractedValue.
func getVal(entry TimelineEntry) float64 {
	if len(entry.Values) == 0 {
		return 0.0
	}
	rawVal := entry.Values[0].ExtractedValue
	if rawVal == nil {
		return 0.0
	}
	switch v := rawVal.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		var parsed float64
		if _, parseErr := fmt.Sscanf(v, "%f", &parsed); parseErr == nil {
			return parsed
		}
	}
	return 0.0
}

// calculateTrendScoreAndDirection computes the score and direction from the time-series.
func calculateTrendScoreAndDirection(timeline []TimelineEntry) (float64, string, error) {
	n := len(timeline)
	if n == 0 {
		return 0.0, "flat", nil
	}

	recentCount := 4
	if recentCount > n {
		recentCount = n
	}
	olderCount := 8
	if olderCount > n-recentCount {
		olderCount = n - recentCount
	}

	var recentSum float64
	for i := n - recentCount; i < n; i++ {
		recentSum += getVal(timeline[i])
	}

	var olderSum float64
	for i := n - recentCount - olderCount; i < n-recentCount; i++ {
		olderSum += getVal(timeline[i])
	}

	recentAvg := 0.0
	if recentCount > 0 {
		recentAvg = recentSum / float64(recentCount)
	}

	olderAvg := 0.0
	if olderCount > 0 {
		olderAvg = olderSum / float64(olderCount)
	}

	rawOlderAvg := olderAvg
	if olderAvg < 1.0 {
		olderAvg = 1.0
	}

	score := recentAvg / olderAvg
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	direction := "flat"
	if rawOlderAvg > 0 {
		ratio := recentAvg / rawOlderAvg
		if ratio > 1.05 {
			direction = "rising"
		} else if ratio < 0.95 {
			direction = "declining"
		}
	}

	return score, direction, nil
}

// GetVelocity queries SerpAPI for trend velocity score and direction.
func (s *SerpAPITrends) GetVelocity(ctx context.Context, keyword string, period string) (float64, string, error) {
	if s.apiKey == "" {
		return 0.0, "flat", nil
	}

	data, err := s.fetchSerpAPIData(ctx, keyword, period)
	if err != nil {
		return 0.0, "", err
	}

	return calculateTrendScoreAndDirection(data.InterestOverTime.TimelineData)
}

// TrendsService coordinates the execution of multiple TrendSources.
type TrendsService struct {
	cfg     *config.Config
	client  *http.Client
	sources map[string]TrendSource
}

// NewTrendsService creates a new TrendsService.
func NewTrendsService(cfg *config.Config, client *http.Client) *TrendsService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	sources := map[string]TrendSource{
		"amazon": NewAmazonAutocomplete(client),
		"serp":   NewSerpAPITrends(client, cfg.Plugins.Trends.SerpAPIKey),
	}

	return &TrendsService{
		cfg:     cfg,
		client:  client,
		sources: sources,
	}
}

// SetHTTPClient sets a custom HTTP client (useful for unit testing).
func (ts *TrendsService) SetHTTPClient(client *http.Client) {
	ts.client = client
	if amazon, ok := ts.sources["amazon"].(*AmazonAutocomplete); ok {
		amazon.client = client
	}
	if serp, ok := ts.sources["serp"].(*SerpAPITrends); ok {
		serp.client = client
	}
}

// ScoreNiche gathers results from requested sources and merges them.
func (ts *TrendsService) ScoreNiche(ctx context.Context, keyword string, limit int, sourceNames []string) ([]Candidate, error) {
	if len(sourceNames) == 0 {
		sourceNames = []string{"amazon", "serp"}
	}

	merged := make(map[string]*Candidate)

	for _, srcName := range sourceNames {
		src, ok := ts.sources[srcName]
		if !ok {
			continue
		}

		cands, err := src.Score(ctx, keyword, limit)
		if err != nil {
			return nil, fmt.Errorf("source %s failed: %w", srcName, err)
		}

		for _, cand := range cands {
			normKeyword := strings.ToLower(strings.TrimSpace(cand.Keyword))
			if normKeyword == "" {
				continue
			}

			if existing, ok := merged[normKeyword]; ok {
				existing.Completions += cand.Completions
				if cand.TrendScore > existing.TrendScore {
					existing.TrendScore = cand.TrendScore
				}
			} else {
				merged[normKeyword] = &Candidate{
					Keyword:     cand.Keyword, // preserve original casing
					Completions: cand.Completions,
					TrendScore:  cand.TrendScore,
				}
			}
		}
	}

	res := make([]Candidate, 0, len(merged))
	for _, cand := range merged {
		res = append(res, *cand)
	}

	return res, nil
}

// GetTrendVelocity directly calls the SerpAPI backend to get trend velocity.
func (ts *TrendsService) GetTrendVelocity(ctx context.Context, keyword string, period string) (float64, string, error) {
	serp, ok := ts.sources["serp"].(*SerpAPITrends)
	if !ok {
		return 0.0, "flat", fmt.Errorf("SerpAPI source not registered")
	}

	return serp.GetVelocity(ctx, keyword, period)
}
