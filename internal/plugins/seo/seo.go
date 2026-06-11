package seo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"powerword/internal/config"
	"powerword/internal/llm"
)

var (
	asinRx          = regexp.MustCompile(`/(?:dp|gp/product)/([A-Z0-9]{10})`)
	titleRx         = regexp.MustCompile(`id="productTitle"[^>]*>\s*([^<]+?)\s*</span>`)
	subtitleRx      = regexp.MustCompile(`id="productSubtitle"[^>]*>\s*([^<]+?)\s*</span>`)
	descRx          = regexp.MustCompile(`bookDescription_feature_div[^>]*>.*?<noscript>\s*<div>\s*([\s\S]+?)\s*</div>\s*</noscript>`)
	authorRx        = regexp.MustCompile(`class="[^"]*author[^"]*"[^>]*>.*?<a[^>]*>([^<]+)</a>`)
	authorRx2       = regexp.MustCompile(`contributorNameID[^>]*>([^<]+)`)
	priceRx         = regexp.MustCompile(`<span class="a-price-whole">([0-9.,]+)`)
	priceFractionRx = regexp.MustCompile(`<span class="a-price-fraction">([0-9]+)`)
	priceAltRx      = regexp.MustCompile(`\$([0-9]+\.[0-9]{2})`)
	ratingRx        = regexp.MustCompile(`([0-9.]+)\s+out of 5 stars`)
	pubDateRx       = regexp.MustCompile(`Publication date\s*:\s*\x{200e}?([A-Za-z0-9\s,]+)`)
	publisherRx     = regexp.MustCompile(`Publisher\s*:\s*\x{200e}?([A-Za-z0-9\s,.-]+)`)
	bsrRx           = regexp.MustCompile(`(?:Best Sellers Rank|Best Seller Rank):\s*#([0-9,]+)`)
	bsrAltRx        = regexp.MustCompile(`#([0-9,]+)\s+in\s+(?:Books|Kindle Store|Audible Books & Originals)`)
	reviewsRx       = regexp.MustCompile(`id="acrCustomerReviewText"[^>]*>\s*([0-9,]+)`)
	reviewsAltRx    = regexp.MustCompile(`([0-9,]+)\s+ratings`)
	htmlTagRx       = regexp.MustCompile(`<[^>]*>`)
)

// CompetitorBook represents a competitor's metadata parsed from Amazon.
type CompetitorBook struct {
	ASIN            string  `json:"asin"`
	Title           string  `json:"title"`
	Subtitle        string  `json:"subtitle,omitempty"`
	Author          string  `json:"author,omitempty"`
	Price           string  `json:"price,omitempty"`
	BSR             int     `json:"bsr,omitempty"`
	Rating          float64 `json:"rating,omitempty"`
	ReviewsCount    int     `json:"reviews_count,omitempty"`
	Description     string  `json:"description,omitempty"`
	PublicationDate string  `json:"publication_date,omitempty"`
	Publisher       string  `json:"publisher,omitempty"`
}

// NicheAnalysisResult contains the results of niche keywords and competitor research.
type NicheAnalysisResult struct {
	Query             string           `json:"query"`
	SearchSuggestions []string         `json:"search_suggestions"`
	Competitors       []CompetitorBook `json:"competitors"`
}

// ListingResult holds the LLM-optimized title, subtitle, keywords, and description.
type ListingResult struct {
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Keywords    []string `json:"keywords"` // exactly 7 search keywords
	Description string   `json:"description"`
}

// SEOService implements KDP SEO scraping, caching, and listing generation.
type SEOService struct {
	cfg       *config.Config
	client    *http.Client
	llmClient llm.LLMClient
}

// NewSEOService creates an instance of SEOService.
func NewSEOService(cfg *config.Config) *SEOService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &SEOService{
		cfg: cfg,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SetHTTPClient sets a custom HTTP client (useful for unit testing).
func (s *SEOService) SetHTTPClient(client *http.Client) {
	s.client = client
}

// SetLLMClient sets a custom LLM client (useful for unit testing).
func (s *SEOService) SetLLMClient(llmClient llm.LLMClient) {
	s.llmClient = llmClient
}

// getCacheDir returns the directory used for caching HTTP responses.
func (s *SEOService) getCacheDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		dir := filepath.Join(home, ".cache", "powerword", "seo-cache")
		if err := os.MkdirAll(dir, 0700); err == nil {
			return dir
		}
	}
	dir := filepath.Join(os.TempDir(), "powerword-seo-cache")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

// cacheKey returns a hashed filename for caching.
func (s *SEOService) cacheKey(val string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(val))
	return hex.EncodeToString(h.Sum(nil)) + ".json"
}

// readFromCache returns cached bytes if they exist and are within TTL.
func (s *SEOService) readFromCache(key string) ([]byte, bool) {
	ttlHours := s.cfg.Plugins.SEO.CacheTTLHours
	if ttlHours < 0 {
		return nil, false
	}
	if ttlHours == 0 {
		ttlHours = 4.0
	}

	cacheFile := filepath.Join(s.getCacheDir(), key)
	info, err := os.Stat(cacheFile)
	if err != nil {
		return nil, false
	}

	if time.Since(info.ModTime()).Hours() > ttlHours {
		return nil, false
	}

	//nolint:gosec // cacheFile is clean and constructed from a SHA-256 hash
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	return data, true
}

// writeToCache saves data to cache.
func (s *SEOService) writeToCache(key string, data []byte) {
	if s.cfg.Plugins.SEO.CacheTTLHours < 0 {
		return
	}
	cacheFile := filepath.Join(s.getCacheDir(), key)
	_ = os.WriteFile(cacheFile, data, 0600)
}

// waitBeforeRequest throttles queries in a context-aware way.
func (s *SEOService) waitBeforeRequest(ctx context.Context) error {
	rateLimitMs := s.cfg.Plugins.SEO.RateLimitMS
	if rateLimitMs <= 0 {
		rateLimitMs = 500
	}
	return sleepContext(ctx, time.Duration(rateLimitMs)*time.Millisecond)
}

// sleepContext sleeps in a context-aware manner, returning early if the context is cancelled.
func sleepContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

// executeRequestAttempt performs a single HTTP request attempt for getWithRetry.
func (s *SEOService) executeRequestAttempt(ctx context.Context, urlStr string, backoff time.Duration) ([]byte, bool, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, false, backoff, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := s.client.Do(req)
	if err != nil {
		if sleepErr := sleepContext(ctx, backoff); sleepErr != nil {
			return nil, false, backoff, sleepErr
		}
		return nil, true, backoff * 2, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == 503 || resp.StatusCode == 429 {
		err = fmt.Errorf("amazon rate limited with status: %d", resp.StatusCode)
		if sleepErr := sleepContext(ctx, backoff); sleepErr != nil {
			return nil, false, backoff, sleepErr
		}
		return nil, true, backoff * 2, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, backoff, fmt.Errorf("invalid status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, backoff, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, false, backoff, nil
}

// getWithRetry retrieves URL body with caching, retries, and rate limit backoff.
func (s *SEOService) getWithRetry(ctx context.Context, urlStr string) ([]byte, error) {
	key := s.cacheKey(urlStr)
	if data, ok := s.readFromCache(key); ok {
		return data, nil
	}

	if err := s.waitBeforeRequest(ctx); err != nil {
		return nil, err
	}

	var lastErr error
	rateLimitMs := s.cfg.Plugins.SEO.RateLimitMS
	if rateLimitMs <= 0 {
		rateLimitMs = 500
	}
	backoff := time.Duration(rateLimitMs) * time.Millisecond
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		body, retry, newBackoff, err := s.executeRequestAttempt(ctx, urlStr, backoff)
		backoff = newBackoff
		if err != nil {
			if retry {
				lastErr = err
				continue
			}
			return nil, err
		}
		s.writeToCache(key, body)
		return body, nil
	}

	return nil, fmt.Errorf("max retries reached: %w", lastErr)
}

// FetchSuggestions retrieves search autocomplete queries.
func (s *SEOService) FetchSuggestions(ctx context.Context, query string) ([]string, error) {
	escapedQuery := url.QueryEscape(query)
	u := fmt.Sprintf("https://completion.amazon.com/search/complete?search-alias=stripbooks&client=amazon-search-ui&mkt=1&q=%s", escapedQuery)

	body, err := s.getWithRetry(ctx, u)
	if err != nil {
		return []string{}, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return []string{}, fmt.Errorf("failed to parse autocomplete response: %w", err)
	}

	if len(raw) > 1 {
		var suggestions []string
		if err := json.Unmarshal(raw[1], &suggestions); err != nil {
			return []string{}, fmt.Errorf("failed to parse suggestions list: %w", err)
		}
		if suggestions == nil {
			return []string{}, nil
		}
		return suggestions, nil
	}

	return []string{}, nil
}

// ExtractASINs pulls unique Amazon ASINs from HTML content using a regex pattern.
func ExtractASINs(htmlContent string) []string {
	matches := asinRx.FindAllStringSubmatch(htmlContent, -1)

	seen := make(map[string]bool)
	var asins []string
	for _, match := range matches {
		if len(match) > 1 {
			asin := match[1]
			if !seen[asin] {
				seen[asin] = true
				asins = append(asins, asin)
			}
		}
	}
	return asins
}

// ParseProductPage parses details from a product page.
func (s *SEOService) ParseProductPage(asin string, htmlContent string) CompetitorBook {
	book := CompetitorBook{
		ASIN: asin,
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err == nil {
		if titleNode := findNodeByID(doc, "productTitle"); titleNode != nil {
			book.Title = getElementText(titleNode)
		}
		if subtitleNode := findNodeByID(doc, "productSubtitle"); subtitleNode != nil {
			book.Subtitle = getElementText(subtitleNode)
		}
		book.Description = extractDescription(doc)
	}

	if book.Title == "" {
		if m := titleRx.FindStringSubmatch(htmlContent); len(m) > 1 {
			book.Title = strings.TrimSpace(m[1])
		}
	}
	if book.Subtitle == "" {
		if m := subtitleRx.FindStringSubmatch(htmlContent); len(m) > 1 {
			book.Subtitle = strings.TrimSpace(m[1])
		}
	}
	if book.Description == "" {
		if m := descRx.FindStringSubmatch(htmlContent); len(m) > 1 {
			book.Description = strings.TrimSpace(m[1])
		}
	}

	book.Author = parseAuthor(htmlContent)
	book.Price = parsePrice(htmlContent)
	book.BSR = parseBSR(htmlContent)
	book.ReviewsCount = parseReviewsCount(htmlContent)
	book.Rating = parseRating(htmlContent)
	book.PublicationDate = parsePublicationDate(htmlContent)
	book.Publisher = parsePublisher(htmlContent)

	book.Description = stripHTML(book.Description)
	return book
}

func parseAuthor(htmlContent string) string {
	if m := authorRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m2 := authorRx2.FindStringSubmatch(htmlContent); len(m2) > 1 {
		return strings.TrimSpace(m2[1])
	}
	return ""
}

func parsePrice(htmlContent string) string {
	if m := priceRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		fraction := "00"
		if mf := priceFractionRx.FindStringSubmatch(htmlContent); len(mf) > 1 {
			fraction = mf[1]
		}
		whole := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(m[1]), "."), ",")
		return "$" + whole + "." + fraction
	}
	if m2 := priceAltRx.FindStringSubmatch(htmlContent); len(m2) > 1 {
		return "$" + m2[1]
	}
	return ""
}

func parseRating(htmlContent string) float64 {
	if m := ratingRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		if rate, err := strconv.ParseFloat(m[1], 64); err == nil {
			return rate
		}
	}
	return 0.0
}

func parsePublicationDate(htmlContent string) string {
	if m := pubDateRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parsePublisher(htmlContent string) string {
	if m := publisherRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// parseBSR parses the best seller rank from Amazon product page HTML.
func parseBSR(htmlContent string) int {
	if m := bsrRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		val := strings.ReplaceAll(m[1], ",", "")
		if rank, err := strconv.Atoi(val); err == nil {
			return rank
		}
	}
	if m2 := bsrAltRx.FindStringSubmatch(htmlContent); len(m2) > 1 {
		val := strings.ReplaceAll(m2[1], ",", "")
		if rank, err := strconv.Atoi(val); err == nil {
			return rank
		}
	}
	return 0
}

// parseReviewsCount parses the number of ratings/reviews from Amazon product page HTML.
func parseReviewsCount(htmlContent string) int {
	if m := reviewsRx.FindStringSubmatch(htmlContent); len(m) > 1 {
		val := strings.ReplaceAll(m[1], ",", "")
		if count, err := strconv.Atoi(val); err == nil {
			return count
		}
	}
	if m2 := reviewsAltRx.FindStringSubmatch(htmlContent); len(m2) > 1 {
		val := strings.ReplaceAll(m2[1], ",", "")
		if count, err := strconv.Atoi(val); err == nil {
			return count
		}
	}
	return 0
}

// searchCompetitorASINs queries Amazon search results to extract ASINs.
func (s *SEOService) searchCompetitorASINs(ctx context.Context, query string) []string {
	escapedQuery := url.QueryEscape(query)
	searchURL := fmt.Sprintf("https://www.amazon.com/s?k=%s&i=stripbooks", escapedQuery)
	body, err := s.getWithRetry(ctx, searchURL)
	if err != nil {
		return nil
	}
	extracted := ExtractASINs(string(body))
	if len(extracted) > 5 {
		extracted = extracted[:5]
	}
	return extracted
}

// AnalyzeNiche evaluates keywords and competitors.
func (s *SEOService) AnalyzeNiche(ctx context.Context, query string, asins []string) (*NicheAnalysisResult, error) {
	var suggestions []string
	var err error
	if query != "" {
		suggestions, err = s.FetchSuggestions(ctx, query)
		if err != nil {
			suggestions = []string{}
		}
	}

	resolvedASINs := make([]string, 0)
	if len(asins) > 0 {
		resolvedASINs = append(resolvedASINs, asins...)
	} else if query != "" {
		resolvedASINs = s.searchCompetitorASINs(ctx, query)
	}

	competitors := make([]CompetitorBook, 0)
	for _, asin := range resolvedASINs {
		productURL := fmt.Sprintf("https://www.amazon.com/dp/%s", asin)
		body, err := s.getWithRetry(ctx, productURL)
		if err != nil {
			continue
		}
		book := s.ParseProductPage(asin, string(body))
		competitors = append(competitors, book)
	}

	return &NicheAnalysisResult{
		Query:             query,
		SearchSuggestions: suggestions,
		Competitors:       competitors,
	}, nil
}

// buildListingPrompt constructs the LLM prompt for listing generation to keep GenerateListing brief.
func buildListingPrompt(niche string, targetAudience string, seedKeywords []string, bookType string, competitorData string) string {
	var sb strings.Builder
	_, _ = sb.WriteString("You are an expert Amazon KDP SEO and Listing copywriter. ")
	_, _ = sb.WriteString("Generate an optimized title, subtitle, exactly seven search keywords (or keyword phrases), and a HTML/Markdown description for a book in the following niche.\n\n")
	_, _ = fmt.Fprintf(&sb, "Niche/Topic: %s\n", niche)
	if targetAudience != "" {
		_, _ = fmt.Fprintf(&sb, "Target Audience: %s\n", targetAudience)
	}
	if bookType != "" {
		_, _ = fmt.Fprintf(&sb, "Book Type: %s\n", bookType)
	}
	if len(seedKeywords) > 0 {
		_, _ = fmt.Fprintf(&sb, "Seed/Suggested Keywords to weave in: %s\n", strings.Join(seedKeywords, ", "))
	}
	if competitorData != "" {
		_, _ = fmt.Fprintf(&sb, "Competitor Context: %s\n", competitorData)
	}

	_, _ = sb.WriteString("\nRules:\n")
	_, _ = sb.WriteString("1. The title must be catchy and relevant.\n")
	_, _ = sb.WriteString("2. The subtitle must expand on the topic using key search terms without keyword-stuffing.\n")
	_, _ = sb.WriteString("3. The search keywords must be exactly 7 keyword phrases/terms, each under 50 characters, optimized for Amazon's backend search boxes.\n")
	_, _ = sb.WriteString("4. The description must be a compelling, beautifully formatted marketing description with bullet points (using HTML or standard markdown, e.g. <b>, <ul>, <li> tags) to optimize for conversion.\n")
	_, _ = sb.WriteString("5. Return the result STRICTLY as a JSON object with this exact structure:\n")
	_, _ = sb.WriteString("{\n")
	_, _ = sb.WriteString("  \"title\": \"string\",\n")
	_, _ = sb.WriteString("  \"subtitle\": \"string\",\n")
	_, _ = sb.WriteString("  \"keywords\": [\"string\", \"string\", ...],\n")
	_, _ = sb.WriteString("  \"description\": \"string\"\n")
	_, _ = sb.WriteString("}\n")
	_, _ = sb.WriteString("Do not include any extra text, markdown code blocks (like ```json), or explanation outside of the JSON object.\n")
	return sb.String()
}

// GenerateListing creates optimized KDP listing using configured LLM client.
func (s *SEOService) GenerateListing(ctx context.Context, niche string, targetAudience string, seedKeywords []string, bookType string, competitorData string) (*ListingResult, error) {
	llmClient := s.llmClient
	var err error
	if llmClient == nil {
		llmClient, err = llm.NewClient(s.cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM client: %w", err)
		}
	}

	prompt := buildListingPrompt(niche, targetAudience, seedKeywords, bookType, competitorData)

	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: "You are a professional Amazon KDP SEO listing generator that ONLY outputs raw JSON matching the requested schema.",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	resp, err := llmClient.Generate(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate listing with LLM: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}
	content = strings.TrimSpace(content)

	var result ListingResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse generated listing JSON: %w (raw response: %s)", err, content)
	}

	var validKeywords []string
	for _, kw := range result.Keywords {
		trimmed := strings.TrimSpace(kw)
		if trimmed != "" {
			validKeywords = append(validKeywords, trimmed)
		}
	}

	if len(validKeywords) != 7 {
		return nil, fmt.Errorf("LLM generated %d valid keywords, but KDP requires exactly 7 search keywords (received: %v)", len(validKeywords), result.Keywords)
	}
	result.Keywords = validKeywords

	return &result, nil
}

// DOM traversal helpers

func findNodeByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == id {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNodeByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func findNodeByClass(n *html.Node, className string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "class" && matchClasses(attr.Val, className) {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNodeByClass(c, className); found != nil {
			return found
		}
	}
	return nil
}

func matchClasses(attrVal, className string) bool {
	reqClasses := strings.Fields(className)
	if len(reqClasses) == 0 {
		return false
	}
	actualClasses := strings.Fields(attrVal)

	actualMap := make(map[string]bool)
	for _, c := range actualClasses {
		actualMap[c] = true
	}

	for _, rc := range reqClasses {
		if !actualMap[rc] {
			return false
		}
	}
	return true
}

func getElementText(n *html.Node) string {
	var sb strings.Builder
	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(n)
	return strings.TrimSpace(sb.String())
}

func extractDescription(n *html.Node) string {
	descDiv := findNodeByID(n, "bookDescription_feature_div")
	if descDiv == nil {
		descDiv = findNodeByClass(n, "a-section a-spacing-small a-padding-small")
	}
	if descDiv != nil {
		return getElementText(descDiv)
	}
	return ""
}

func stripHTML(s string) string {
	return strings.TrimSpace(htmlTagRx.ReplaceAllString(s, ""))
}
