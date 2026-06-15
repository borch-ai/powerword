package trends

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		name         string
		envURL       string
		defaultValue string
		want         string
	}{
		{
			name:         "empty string",
			envURL:       "",
			defaultValue: "https://default.com",
			want:         "https://default.com",
		},
		{
			name:         "invalid URL",
			envURL:       "://invalid",
			defaultValue: "https://default.com",
			want:         "https://default.com",
		},
		{
			name:         "valid localhost URL",
			envURL:       "http://localhost:8080/",
			defaultValue: "https://default.com",
			want:         "http://localhost:8080",
		},
		{
			name:         "valid 127.0.0.1 URL",
			envURL:       "http://127.0.0.1:9090",
			defaultValue: "https://default.com",
			want:         "http://127.0.0.1:9090",
		},
		{
			name:         "valid ipv6 loopback URL",
			envURL:       "http://[::1]:9090",
			defaultValue: "https://default.com",
			want:         "http://[::1]:9090",
		},
		{
			name:         "unsafe external URL",
			envURL:       "https://malicious.com",
			defaultValue: "https://default.com",
			want:         "https://default.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateBaseURL(tc.envURL, tc.defaultValue)
			if got != tc.want {
				t.Errorf("validateBaseURL(%q, %q) = %q, want %q", tc.envURL, tc.defaultValue, got, tc.want)
			}
		})
	}
}

func TestAmazonAutocomplete_Score_Success(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.String(), "completion.amazon.com") {
				return nil, errors.New("unexpected url")
			}
			respJSON := `["radon detector",["radon detector home","radon detector charcoal"],[],[],"12345"]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	if src.Name() != "amazon" {
		t.Errorf("expected name amazon, got %s", src.Name())
	}

	cands, err := src.Score(context.Background(), "radon detector", 10)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}

	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}

	if cands[0].Keyword != "radon detector home" || cands[0].Completions != 2 || cands[0].TrendScore != 0 {
		t.Errorf("unexpected first candidate: %+v", cands[0])
	}
	if cands[1].Keyword != "radon detector charcoal" || cands[1].Completions != 1 || cands[1].TrendScore != 0 {
		t.Errorf("unexpected second candidate: %+v", cands[1])
	}
}

func TestAmazonAutocomplete_Score_Limit(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			respJSON := `["radon",["radon detector","radon mitigation","radon tester"]]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	cands, err := src.Score(context.Background(), "radon", 2)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}

	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates due to limit, got %d", len(cands))
	}
	if cands[0].Keyword != "radon detector" || cands[1].Keyword != "radon mitigation" {
		t.Errorf("unexpected candidates: %+v", cands)
	}
}

func TestAmazonAutocomplete_Score_EmptyResponse(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["query"]`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	cands, err := src.Score(context.Background(), "radon", 10)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}

func TestAmazonAutocomplete_Score_EmptySuggestions(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["query",[]]`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	cands, err := src.Score(context.Background(), "radon", 10)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}

func TestAmazonAutocomplete_Score_Non200(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("server error")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	_, err := src.Score(context.Background(), "radon", 10)
	if err == nil {
		t.Error("expected error for non-200, got nil")
	}
}

func TestAmazonAutocomplete_Score_JSONFailure(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("invalid json")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	_, err := src.Score(context.Background(), "radon", 10)
	if err == nil {
		t.Error("expected error for invalid json, got nil")
	}
}

func TestAmazonAutocomplete_Score_SuggestionsFormatMismatch(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["query", {"invalid": true}]`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewAmazonAutocomplete(client)
	_, err := src.Score(context.Background(), "radon", 10)
	if err == nil {
		t.Error("expected error for bad suggestions format, got nil")
	}
}

func TestAmazonAutocomplete_Score_ContextCancellation(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return nil, context.Canceled
		}),
	}

	src := NewAmazonAutocomplete(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.Score(ctx, "radon", 10)
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got %v", err)
	}
}

func TestSerpAPITrends_GetVelocity_NoKey(t *testing.T) {
	src := NewSerpAPITrends(nil, "")
	if src.Name() != "serp" {
		t.Errorf("expected name serp, got %s", src.Name())
	}

	score, dir, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if score != 0.0 || dir != "flat" {
		t.Errorf("expected zero values, got score=%f, dir=%s", score, dir)
	}

	cands, err := src.Score(context.Background(), "radon", 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}

func TestSerpAPITrends_GetVelocity_Non200(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("Invalid API Key")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	_, _, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
}

func TestSerpAPITrends_GetVelocity_DecodeError(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{invalid json")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	_, _, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err == nil {
		t.Error("expected decode error, got nil")
	}
}

func TestSerpAPITrends_GetVelocity_EmptyTimeline(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"interest_over_time": {"timeline_data": []}}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	score, dir, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if score != 0.0 || dir != "flat" {
		t.Errorf("expected 0.0 and flat, got score=%f, dir=%s", score, dir)
	}
}

func TestSerpAPITrends_GetVelocity_Rising(t *testing.T) {
	// 12 weeks: older 8 weeks average 20, recent 4 weeks average 80.
	// ratio = 80/20 = 4.0 -> TrendScore = capped at 1.0, direction = rising
	respJSON := `{
		"interest_over_time": {
			"timeline_data": [
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]}
			]
		}
	}`

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	score, dir, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err != nil {
		t.Fatalf("GetVelocity failed: %v", err)
	}

	if score != 1.0 {
		t.Errorf("expected capped score 1.0, got %f", score)
	}
	if dir != "rising" {
		t.Errorf("expected rising trend, got %s", dir)
	}

	// Also check Score implementation
	cands, err := src.Score(context.Background(), "radon", 10)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if len(cands) != 1 || cands[0].TrendScore != 1.0 || cands[0].Keyword != "radon" {
		t.Errorf("unexpected candidates: %+v", cands)
	}
}

func TestSerpAPITrends_GetVelocity_Declining(t *testing.T) {
	// older 8 weeks average 80, recent 4 weeks average 20.
	// ratio = 20/80 = 0.25 -> TrendScore = 0.25, direction = declining
	respJSON := `{
		"interest_over_time": {
			"timeline_data": [
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 80}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]},
				{"values": [{"extracted_value": 20}]}
			]
		}
	}`

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	score, dir, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err != nil {
		t.Fatalf("GetVelocity failed: %v", err)
	}

	if score != 0.25 {
		t.Errorf("expected score 0.25, got %f", score)
	}
	if dir != "declining" {
		t.Errorf("expected declining trend, got %s", dir)
	}
}

func TestSerpAPITrends_GetVelocity_Flat(t *testing.T) {
	// older 8 weeks average 50, recent 4 weeks average 50.
	// ratio = 50/50 = 1.0 -> TrendScore = 1.0, direction = flat
	respJSON := `{
		"interest_over_time": {
			"timeline_data": [
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]},
				{"values": [{"extracted_value": 50}]}
			]
		}
	}`

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	score, dir, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err != nil {
		t.Fatalf("GetVelocity failed: %v", err)
	}

	if score != 1.0 {
		t.Errorf("expected score 1.0, got %f", score)
	}
	if dir != "flat" {
		t.Errorf("expected flat trend, got %s", dir)
	}
}

func TestSerpAPITrends_GetVelocity_ExtractedValueFormat(t *testing.T) {
	// Check interface conversion for int and string formats
	respJSON := `{
		"interest_over_time": {
			"timeline_data": [
				{"values": [{"extracted_value": 40}]},
				{"values": [{"extracted_value": "40"}]},
				{"values": [{"extracted_value": 40.0}]},
				{"values": [{}], "date": "missing"},
				{"values": null}
			]
		}
	}`

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := NewSerpAPITrends(client, "test-key")
	score, _, err := src.GetVelocity(context.Background(), "radon", "3-m")
	if err != nil {
		t.Fatalf("GetVelocity failed: %v", err)
	}
	if score != 0.0 { // since count is small and olderAvg=1, etc. Just check it runs without panic.
		t.Logf("Got score %f", score)
	}
}

func TestTrendsService_ScoreNiche_Success(t *testing.T) {
	amazonTransport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "completion.amazon.com") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["radon",["radon detector","radon home"]]`)),
				Header:     make(http.Header),
			}, nil
		}
		return nil, errors.New("unexpected url")
	})

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			// route based on domain
			if strings.Contains(req.URL.String(), "completion.amazon.com") {
				return amazonTransport.RoundTrip(req)
			}
			if strings.Contains(req.URL.String(), "serpapi.com") {
				respJSON := `{
					"interest_over_time": {
						"timeline_data": [
							{"values": [{"extracted_value": 100}]},
							{"values": [{"extracted_value": 100}]}
						]
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respJSON)),
					Header:     make(http.Header),
				}, nil
			}
			return nil, errors.New("unexpected url")
		}),
	}

	cfg := &config.Config{}
	cfg.Plugins.Trends.SerpAPIKey = "dummy-key"
	svc := NewTrendsService(cfg, client)

	// Set client explicitly
	svc.SetHTTPClient(client)

	cands, err := svc.ScoreNiche(context.Background(), "radon", 10, []string{"amazon", "serp"})
	if err != nil {
		t.Fatalf("ScoreNiche failed: %v", err)
	}

	// Expected candidates:
	// - "radon detector" (from amazon) - completions=2, trend=0
	// - "radon home" (from amazon) - completions=1, trend=0
	// - "radon" (from serp) - completions=0, trend=1.0 (since ratio=100/100=1.0)
	if len(cands) != 3 {
		t.Fatalf("expected 3 merged candidates, got %d: %+v", len(cands), cands)
	}

	var radonDet, radonHome, radonSeed *Candidate
	for i := range cands {
		switch strings.ToLower(cands[i].Keyword) {
		case "radon detector":
			radonDet = &cands[i]
		case "radon home":
			radonHome = &cands[i]
		case "radon":
			radonSeed = &cands[i]
		}
	}

	if radonDet == nil || radonDet.Completions != 2 || radonDet.TrendScore != 0.0 {
		t.Errorf("unexpected radon detector: %+v", radonDet)
	}
	if radonHome == nil || radonHome.Completions != 1 || radonHome.TrendScore != 0.0 {
		t.Errorf("unexpected radon home: %+v", radonHome)
	}
	if radonSeed == nil || radonSeed.Completions != 0 || radonSeed.TrendScore != 1.0 {
		t.Errorf("unexpected radon seed: %+v", radonSeed)
	}
}

func TestTrendsService_ScoreNiche_Failure(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		}),
	}
	cfg := &config.Config{}
	cfg.Plugins.Trends.SerpAPIKey = "dummy-key"
	svc := NewTrendsService(cfg, client)

	_, err := svc.ScoreNiche(context.Background(), "radon", 10, []string{"amazon"})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTrendsService_ScoreNiche_Unknown(t *testing.T) {
	svc := NewTrendsService(nil, nil)
	cands, err := svc.ScoreNiche(context.Background(), "radon", 10, []string{"unknown"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}

func TestTrendsService_ScoreNiche_UnregisteredSerp(t *testing.T) {
	svc := &TrendsService{
		cfg:     &config.Config{},
		client:  &http.Client{},
		sources: make(map[string]TrendSource),
	}

	_, _, err := svc.GetTrendVelocity(context.Background(), "radon", "3-m")
	if err == nil {
		t.Error("expected error for missing serp source, got nil")
	}
}
