package seo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

type mockLLM struct {
	response *llm.Message
	err      error
}

func (m *mockLLM) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Message, error) {
	return m.response, m.err
}

func (m *mockLLM) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockLLM) ListModels(ctx context.Context) ([]string, error) {
	return []string{"mock"}, nil
}

func TestFetchSuggestions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	called := 0
	transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		called++
		if !strings.Contains(req.URL.String(), "completion.amazon.com") {
			return nil, errors.New("unexpected url")
		}
		respJSON := `["existential nursery rhymes",["existential nursery rhymes book","existential nursery rhymes for toddlers"],[],[],"12345"]`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respJSON)),
			Header:     make(http.Header),
		}, nil
	})

	cfg := &config.Config{}
	cfg.Plugins.SEO.CacheTTLHours = -1.0 // Disable cache or force refresh for this test
	cfg.Plugins.SEO.RateLimitMS = 1

	svc := NewSEOService(cfg)
	svc.SetHTTPClient(&http.Client{Transport: transport})

	suggestions, err := svc.FetchSuggestions(context.Background(), "existential nursery rhymes")
	if err != nil {
		t.Fatalf("FetchSuggestions failed: %v", err)
	}

	if len(suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0] != "existential nursery rhymes book" {
		t.Errorf("expected suggestion[0] to be 'existential nursery rhymes book', got %q", suggestions[0])
	}
	if called != 1 {
		t.Errorf("expected 1 network call, got %d", called)
	}
}

func TestExtractASINs(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<a href="/dp/B08X5Z8N21/ref=sr_1_1">Book Title 1</a>
				<a href="/gp/product/1501168001">Book Title 2</a>
				<a href="/dp/B08X5Z8N21">Duplicate Link</a>
				<a href="/other-link">Not an ASIN</a>
			</body>
		</html>
	`
	asins := ExtractASINs(htmlContent)
	if len(asins) != 2 {
		t.Fatalf("expected 2 unique ASINs, got %d: %v", len(asins), asins)
	}
	if asins[0] != "B08X5Z8N21" {
		t.Errorf("expected first ASIN B08X5Z8N21, got %s", asins[0])
	}
	if asins[1] != "1501168001" {
		t.Errorf("expected second ASIN 1501168001, got %s", asins[1])
	}
}

func TestParseProductPage(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<span id="productTitle">Existential Nursery Rhymes</span>
				<span id="productSubtitle">A book of dread for toddlers</span>
				<div id="bookDescription_feature_div">
					<noscript><div>This is a book about nothingness.</div></noscript>
				</div>
				<span class="author">By <a href="/author/John-Doe">John Doe</a></span>
				<span class="a-price-whole">12.</span><span class="a-price-fraction">99</span>
				<div>Best Sellers Rank: #45,123 in Books</div>
				<div>4.6 out of 5 stars</div>
				<span id="acrCustomerReviewText">128 ratings</span>
				<div>Publication date: October 20, 2025</div>
				<div>Publisher: Antigravity Press</div>
			</body>
		</html>
	`

	svc := NewSEOService(nil)
	book := svc.ParseProductPage("B08X5Z8N21", htmlContent)

	if book.Title != "Existential Nursery Rhymes" {
		t.Errorf("expected Title 'Existential Nursery Rhymes', got %q", book.Title)
	}
	if book.Subtitle != "A book of dread for toddlers" {
		t.Errorf("expected Subtitle 'A book of dread for toddlers', got %q", book.Subtitle)
	}
	if book.Description != "This is a book about nothingness." {
		t.Errorf("expected Description 'This is a book about nothingness.', got %q", book.Description)
	}
	if book.Author != "John Doe" {
		t.Errorf("expected Author 'John Doe', got %q", book.Author)
	}
	if book.Price != "$12.99" {
		t.Errorf("expected Price '$12.99', got %q", book.Price)
	}
	if book.BSR != 45123 {
		t.Errorf("expected BSR 45123, got %d", book.BSR)
	}
	if book.Rating != 4.6 {
		t.Errorf("expected Rating 4.6, got %f", book.Rating)
	}
	if book.ReviewsCount != 128 {
		t.Errorf("expected ReviewsCount 128, got %d", book.ReviewsCount)
	}
	if book.PublicationDate != "October 20, 2025" {
		t.Errorf("expected PublicationDate 'October 20, 2025', got %q", book.PublicationDate)
	}
	if book.Publisher != "Antigravity Press" {
		t.Errorf("expected Publisher 'Antigravity Press', got %q", book.Publisher)
	}
}

func TestAnalyzeNiche(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	searchCalled := false
	productCalled := false

	transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		u := req.URL.String()
		if strings.Contains(u, "completion.amazon.com") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["existential",["existential book"]]`)),
				Header:     make(http.Header),
			}, nil
		}
		if strings.Contains(u, "amazon.com/s?") {
			searchCalled = true
			resp := `<html><body><a href="/dp/B08X5Z8N21">Link</a></body></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(resp)),
				Header:     make(http.Header),
			}, nil
		}
		if strings.Contains(u, "amazon.com/dp/") {
			productCalled = true
			resp := `<html><body><span id="productTitle">Test Title</span></body></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(resp)),
				Header:     make(http.Header),
			}, nil
		}
		return nil, errors.New("unexpected call")
	})

	cfg := &config.Config{}
	cfg.Plugins.SEO.CacheTTLHours = -1.0
	cfg.Plugins.SEO.RateLimitMS = 1

	svc := NewSEOService(cfg)
	svc.SetHTTPClient(&http.Client{Transport: transport})

	res, err := svc.AnalyzeNiche(context.Background(), "existential", nil)
	if err != nil {
		t.Fatalf("AnalyzeNiche failed: %v", err)
	}

	if !searchCalled {
		t.Error("expected search URL to be called")
	}
	if !productCalled {
		t.Error("expected product detail URL to be called")
	}

	if len(res.SearchSuggestions) != 1 || res.SearchSuggestions[0] != "existential book" {
		t.Errorf("unexpected suggestions: %v", res.SearchSuggestions)
	}
	if len(res.Competitors) != 1 || res.Competitors[0].Title != "Test Title" {
		t.Errorf("unexpected competitors: %v", res.Competitors)
	}
}

func TestGenerateListing(t *testing.T) {
	t.Run("Valid JSON generation", func(t *testing.T) {
		llmResponse := `
		{
			"title": "Optimized Title",
			"subtitle": "Optimized Subtitle",
			"keywords": ["kw1", "kw2", "kw3", "kw4", "kw5", "kw6", "kw7"],
			"description": "Best book description"
		}
		`

		mockL := &mockLLM{
			response: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: llmResponse,
			},
		}

		svc := NewSEOService(nil)
		svc.SetLLMClient(mockL)

		res, err := svc.GenerateListing(context.Background(), "Existential Rhymes", "Parents", []string{"seed"}, "Book", "Competitor")
		if err != nil {
			t.Fatalf("GenerateListing failed: %v", err)
		}

		if res.Title != "Optimized Title" {
			t.Errorf("expected Title 'Optimized Title', got %q", res.Title)
		}
		if len(res.Keywords) != 7 {
			t.Errorf("expected exactly 7 keywords, got %d", len(res.Keywords))
		}
	})

	t.Run("Wrapped markdown code blocks JSON", func(t *testing.T) {
		llmResponse := "```json\n{\n  \"title\": \"Title 2\",\n  \"subtitle\": \"Sub 2\",\n  \"keywords\": [\"a\", \"b\", \"c\", \"d\", \"e\", \"f\", \"g\"],\n  \"description\": \"desc\"\n}\n```"

		mockL := &mockLLM{
			response: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: llmResponse,
			},
		}

		svc := NewSEOService(nil)
		svc.SetLLMClient(mockL)

		res, err := svc.GenerateListing(context.Background(), "Existential Rhymes", "Parents", nil, "Book", "")
		if err != nil {
			t.Fatalf("GenerateListing failed: %v", err)
		}

		if res.Title != "Title 2" {
			t.Errorf("expected Title 'Title 2', got %q", res.Title)
		}
		if len(res.Keywords) != 7 {
			t.Errorf("expected exactly 7 keywords, got %d", len(res.Keywords))
		}
	})

	t.Run("Invalid keywords length or empty keywords fails fast", func(t *testing.T) {
		// Only 2 keywords
		llmResponse := "{\n  \"title\": \"T\",\n  \"subtitle\": \"S\",\n  \"keywords\": [\"a\", \"b\"],\n  \"description\": \"d\"\n}"
		mockL := &mockLLM{
			response: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: llmResponse,
			},
		}

		svc := NewSEOService(nil)
		svc.SetLLMClient(mockL)

		_, err := svc.GenerateListing(context.Background(), "Niche", "", nil, "", "")
		if err == nil {
			t.Error("expected error for invalid keyword count, got nil")
		}

		// 7 keywords but one is empty/whitespace
		llmResponse2 := "{\n  \"title\": \"T\",\n  \"subtitle\": \"S\",\n  \"keywords\": [\"a\", \"b\", \"c\", \"d\", \"e\", \"f\", \"   \"],\n  \"description\": \"d\"\n}"
		mockL2 := &mockLLM{
			response: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: llmResponse2,
			},
		}
		svc.SetLLMClient(mockL2)
		_, err = svc.GenerateListing(context.Background(), "Niche", "", nil, "", "")
		if err == nil {
			t.Error("expected error for empty/whitespace keywords, got nil")
		}
	})
}

func TestCacheAndThrottling(t *testing.T) {
	tempDir := t.TempDir()

	called := 0
	transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		called++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`["query", ["res"]]`)),
			Header:     make(http.Header),
		}, nil
	})

	// Inject a custom homedir or manually use tempDir as the caching path in tests
	// Let's modify the cache dir logic to use temp dir when testing or override home
	t.Setenv("HOME", tempDir)

	cfg := &config.Config{}
	cfg.Plugins.SEO.CacheTTLHours = 1.0
	cfg.Plugins.SEO.RateLimitMS = 1

	svc := NewSEOService(cfg)
	svc.SetHTTPClient(&http.Client{Transport: transport})

	// First call -> hits network
	_, err := svc.FetchSuggestions(context.Background(), "test-cache")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call -> hits cache
	_, err = svc.FetchSuggestions(context.Background(), "test-cache")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if called != 1 {
		t.Errorf("expected exactly 1 network call, got %d", called)
	}
}

func TestGetWithRetryFailures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	called := 0
	transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		called++
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("Busy")),
			Header:     make(http.Header),
		}, nil
	})

	cfg := &config.Config{}
	cfg.Plugins.SEO.CacheTTLHours = -1
	cfg.Plugins.SEO.RateLimitMS = 1

	svc := NewSEOService(cfg)
	svc.SetHTTPClient(&http.Client{Transport: transport})

	_, err := svc.FetchSuggestions(context.Background(), "fail")
	if err == nil {
		t.Error("expected failure due to max retries on 503, got nil")
	}

	if called != 3 {
		t.Errorf("expected 3 retries, got %d", called)
	}
}

func TestParserDescriptionFallback(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<span id="productTitle">Test</span>
				<div class="a-section a-spacing-small a-padding-small">Fallback Description Text</div>
			</body>
		</html>
	`

	svc := NewSEOService(nil)
	book := svc.ParseProductPage("B08X5Z8N21", htmlContent)

	if book.Description != "Fallback Description Text" {
		t.Errorf("expected Fallback description, got %q", book.Description)
	}
}

func TestGetCacheDirFallbacks(t *testing.T) {
	// Create a temp file to act as the HOME dir, preventing directory creation under it
	tempFile, err := os.CreateTemp(t.TempDir(), "home-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = tempFile.Close()

	t.Setenv("HOME", tempFile.Name())
	svc := NewSEOService(nil)
	dir := svc.getCacheDir()
	if !strings.Contains(dir, "powerword-seo-cache") {
		t.Errorf("expected fallback cache dir containing powerword-seo-cache, got %s", dir)
	}
}

func TestParseProductPageRegexFallbacks(t *testing.T) {
	htmlContent := `
		id="productTitle">   Regex Title   </span>
		id="productSubtitle">   Regex Subtitle   </span>
		bookDescription_feature_div> <noscript> <div> Regex Description </div> </noscript>
		contributorNameID>Regex Author<
		$15.50
	`

	svc := NewSEOService(nil)
	book := svc.ParseProductPage("B08X5Z8N21", htmlContent)

	if book.Title != "Regex Title" {
		t.Errorf("expected regex title, got %q", book.Title)
	}
	if book.Subtitle != "Regex Subtitle" {
		t.Errorf("expected regex subtitle, got %q", book.Subtitle)
	}
	if book.Description != "Regex Description" {
		t.Errorf("expected regex description, got %q", book.Description)
	}
	if book.Author != "Regex Author" {
		t.Errorf("expected regex author, got %q", book.Author)
	}
	if book.Price != "$15.50" {
		t.Errorf("expected regex price, got %q", book.Price)
	}
}

func TestCacheExpiration(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cfg := &config.Config{}
	cfg.Plugins.SEO.CacheTTLHours = 1.0
	cfg.Plugins.SEO.RateLimitMS = 1
	svc := NewSEOService(cfg)

	key := svc.cacheKey("expired-key")
	svc.writeToCache(key, []byte("expired-data"))

	cacheFile := filepath.Join(svc.getCacheDir(), key)
	past := time.Now().Add(-5 * time.Hour)
	err := os.Chtimes(cacheFile, past, past)
	if err != nil {
		t.Fatalf("failed to change mod time: %v", err)
	}

	data, ok := svc.readFromCache(key)
	if ok || data != nil {
		t.Error("expected cache to be expired")
	}
}

func TestGenerateListingNewClientError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Model = "gemini-1.5-pro"
	svc := NewSEOService(cfg)

	_, err := svc.GenerateListing(context.Background(), "Niche", "", nil, "", "")
	if err == nil {
		t.Error("expected error due to missing API key, got nil")
	}
}

func TestFetchSuggestionsParsingErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	t.Run("Invalid JSON", func(t *testing.T) {
		transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("invalid")),
				Header:     make(http.Header),
			}, nil
		})
		svc := NewSEOService(nil)
		svc.SetHTTPClient(&http.Client{Transport: transport})
		_, err := svc.FetchSuggestions(context.Background(), "query-invalid")
		if err == nil {
			t.Error("expected unmarshal error")
		}
	})

	t.Run("Short raw array", func(t *testing.T) {
		transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["query"]`)),
				Header:     make(http.Header),
			}, nil
		})
		svc := NewSEOService(nil)
		svc.SetHTTPClient(&http.Client{Transport: transport})
		suggestions, err := svc.FetchSuggestions(context.Background(), "query-short")
		if err != nil {
			t.Fatalf("FetchSuggestions failed: %v", err)
		}
		if len(suggestions) != 0 {
			t.Errorf("expected 0 suggestions, got %v", suggestions)
		}
	})

	t.Run("Invalid suggestions array format", func(t *testing.T) {
		transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["query", {"invalid": 1}]`)),
				Header:     make(http.Header),
			}, nil
		})
		svc := NewSEOService(nil)
		svc.SetHTTPClient(&http.Client{Transport: transport})
		_, err := svc.FetchSuggestions(context.Background(), "query-format")
		if err == nil {
			t.Error("expected suggestions list unmarshal error")
		}
	})

	t.Run("Returns empty initialized slice instead of nil", func(t *testing.T) {
		transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["query"]`)),
				Header:     make(http.Header),
			}, nil
		})
		svc := NewSEOService(nil)
		svc.SetHTTPClient(&http.Client{Transport: transport})
		suggestions, err := svc.FetchSuggestions(context.Background(), "query-short")
		if err != nil {
			t.Fatalf("FetchSuggestions failed: %v", err)
		}
		if suggestions == nil {
			t.Error("expected suggestions to be non-nil slice")
		}
	})
}

func TestAnalyzeNicheSuggestionsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "completion.amazon.com") {
			return nil, errors.New("suggestions failure")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	svc := NewSEOService(nil)
	svc.SetHTTPClient(&http.Client{Transport: transport})

	res, err := svc.AnalyzeNiche(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(res.SearchSuggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %v", res.SearchSuggestions)
	}
}

func TestSleepContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewSEOService(nil)
	err := svc.waitBeforeRequest(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}

	err = sleepContext(ctx, 10*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}
