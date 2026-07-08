package main

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"github.com/borch-ai/powerword/pkg/llm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mockEmbedClient struct {
	embeddings [][]float32
}

func (m *mockEmbedClient) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, opts ...llm.GenerateOption) (*llm.Message, error) {
	return nil, nil
}

func (m *mockEmbedClient) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockEmbedClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockEmbedClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return m.embeddings, nil
}

func TestMemoryAddAndSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memory.json")

	// Set up the mock client to return predictable embeddings.
	// We'll mock it such that query matches text2 perfectly.
	client := &mockEmbedClient{
		embeddings: [][]float32{{1.0, 0.0, 0.0}},
	}

	appServer := &Server{
		client: client,
		dbPath: dbPath,
	}

	// Test adding memory
	addFunc := appServer.handleMemoryAdd()

	addReq1 := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"text":"The quick brown fox","tags":["animal"]}`),
		},
	}
	client.embeddings = [][]float32{{1.0, 0.0, 0.0}}
	res, err := addFunc(context.Background(), addReq1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	addReq2 := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"text":"Jumps over the lazy dog","tags":["action"]}`),
		},
	}
	client.embeddings = [][]float32{{0.0, 1.0, 0.0}}
	res, err = addFunc(context.Background(), addReq2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	// Test searching memory
	searchFunc := appServer.handleMemorySearch()

	searchReq := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"query":"fox jumps","limit":1,"min_similarity":0.5}`),
		},
	}
	// query embedding closer to second item
	client.embeddings = [][]float32{{0.0, 0.9, 0.1}}
	res, err = searchFunc(context.Background(), searchReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	var results []struct {
		Text  string  `json:"text"`
		Score float64 `json:"score"`
	}
	textContent := res.Content[0].(*mcp.TextContent).Text
	if err := json.Unmarshal([]byte(textContent), &results); err != nil {
		t.Fatalf("failed to parse search results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != "Jumps over the lazy dog" {
		t.Errorf("expected 'Jumps over the lazy dog', got %q", results[0].Text)
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if math.Abs(cosineSimilarity(a, b)-1.0) > 1e-6 {
		t.Errorf("expected 1.0, got %f", cosineSimilarity(a, b))
	}

	c := []float32{0, 1, 0}
	if cosineSimilarity(a, c) != 0 {
		t.Errorf("expected 0.0, got %f", cosineSimilarity(a, c))
	}

	d := []float32{0.5, 0.5, 0.5}
	e := []float32{0.5, 0.5, 0.5}
	if math.Abs(cosineSimilarity(d, e)-1.0) > 1e-6 {
		t.Errorf("expected 1.0, got %f", cosineSimilarity(d, e))
	}
}

func TestFileLock(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.json")

	fl1 := NewFileLock(dbPath)
	fl2 := NewFileLock(dbPath)

	if err := fl1.Lock(); err != nil {
		t.Fatalf("expected lock 1 success, got %v", err)
	}

	locked := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(locked)
		if err := fl2.Lock(); err != nil {
			t.Errorf("expected lock 2 success after release, got %v", err)
		}
		fl2.Unlock()
		close(done)
	}()

	<-locked
	// wait a bit to ensure fl2 blocks on Lock
	select {
	case <-done:
		t.Fatal("lock 2 should have blocked while lock 1 is held")
	default:
	}

	fl1.Unlock()

	<-done
}
