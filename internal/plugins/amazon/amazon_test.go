package amazon_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/internal/plugins/amazon"
	"github.com/borch-ai/powerword/pkg/config"
)

func TestAmazonService_MockMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "mock"

	svc := amazon.NewAmazonService(cfg, nil)
	count, err := svc.GetListingCount(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 4200 {
		t.Errorf("expected 4200, got %d", count)
	}
}

func TestAmazonService_EmptyKeyError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = ""

	svc := amazon.NewAmazonService(cfg, nil)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Error("expected error due to empty api key, got nil")
	}
}

func TestAmazonService_NewAmazonService_NilParams(t *testing.T) {
	svc := amazon.NewAmazonService(nil, nil)
	if svc == nil {
		t.Fatal("expected NewAmazonService to return service even with nil arguments")
	}
}

func TestAmazonService_InvalidBaseURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = "::invalid::url::"

	svc := amazon.NewAmazonService(cfg, nil)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Error("expected error due to invalid base URL, got nil")
	}
}

func TestAmazonService_BaseURLMissingScheme(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = "api.scaleserp.com" // missing http/https scheme

	svc := amazon.NewAmazonService(cfg, nil)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Error("expected error due to missing base URL scheme, got nil")
	}
}

func TestAmazonService_BaseURLMissingHost(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = "https://" // missing host

	svc := amazon.NewAmazonService(cfg, nil)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Error("expected error due to missing base URL host, got nil")
	}
}

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestAmazonService_ClientDoError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = "https://api.scaleserp.com"

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network connection refused")
		}),
	}

	svc := amazon.NewAmazonService(cfg, client)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Error("expected HTTP request failure error, got nil")
	}
}

func TestAmazonService_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Unauthorized access using key real-key"))
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = srv.URL

	svc := amazon.NewAmazonService(cfg, nil)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
	if strings.Contains(err.Error(), "real-key") {
		t.Errorf("expected API key to be redacted from error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("expected error message to contain 'REDACTED', got: %v", err)
	}
}

func TestAmazonService_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid-json"))
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = srv.URL

	svc := amazon.NewAmazonService(cfg, nil)
	_, err := svc.GetListingCount(context.Background(), "test")
	if err == nil {
		t.Error("expected JSON decode error, got nil")
	}
}

func TestAmazonService_AlternativeFields(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		want    int
	}{
		{
			name: "SearchResults fallback",
			payload: map[string]interface{}{
				"search_results": map[string]interface{}{
					"total_results": 5678,
				},
			},
			want: 5678,
		},
		{
			name: "TotalResults direct field fallback",
			payload: map[string]interface{}{
				"total_results": 9999,
			},
			want: 9999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tt.payload)
			}))
			defer srv.Close()

			cfg := &config.Config{}
			cfg.Plugins.Amazon.APIKey = "real-key"
			cfg.Plugins.Amazon.BaseURL = srv.URL

			svc := amazon.NewAmazonService(cfg, nil)
			count, err := svc.GetListingCount(context.Background(), "test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != tt.want {
				t.Errorf("expected %d, got %d", tt.want, count)
			}
		})
	}
}

func TestAmazonService_GetListingCount_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("api_key") != "real-key" {
			t.Errorf("unexpected api_key: %s", r.URL.Query().Get("api_key"))
		}
		if r.URL.Query().Get("q") != "radon detector" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}

		w.Header().Set("Content-Type", "application/json")
		payload := map[string]interface{}{
			"search_information": map[string]interface{}{
				"total_results": 1234,
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	cfg.Plugins.Amazon.BaseURL = srv.URL

	svc := amazon.NewAmazonService(cfg, nil)
	count, err := svc.GetListingCount(context.Background(), "radon detector")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1234 {
		t.Errorf("expected 1234, got %d", count)
	}
}
