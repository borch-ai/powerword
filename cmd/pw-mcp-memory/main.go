package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MemoryRecord struct {
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	Embedding []float32 `json:"embedding"`
	Timestamp time.Time `json:"timestamp"`
}

type MemoryStore struct {
	Records []MemoryRecord `json:"records"`
}

type Server struct {
	client  llm.LLMClient
	dbPath  string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	client, err := llm.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create llm client: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home dir: %w", err)
	}

	dbPath := filepath.Join(homeDir, ".local", "share", "powerword", "memory.json")

	appServer := &Server{
		client: client,
		dbPath: dbPath,
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-memory",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "memory_add",
		Description: "Adds a text block and optional tags to the semantic memory store.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description":"The text to remember"},"tags":{"type":"array","items":{"type":"string"},"description":"Optional tags for categorization"}},"required":["text"]}`),
	}, appServer.handleMemoryAdd())

	srv.AddTool(&mcp.Tool{
		Name:        "memory_search",
		Description: "Searches the semantic memory store for the most relevant records based on cosine similarity.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"The search query"},"limit":{"type":"integer","description":"Maximum number of results to return (default 5)"},"min_similarity":{"type":"number","description":"Minimum cosine similarity score (0.0 to 1.0, default 0.0)"}},"required":["query"]}`),
	}, appServer.handleMemorySearch())

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

func (s *Server) loadStore() (*MemoryStore, error) {
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0750); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &MemoryStore{}, nil
		}
		return nil, err
	}

	var store MemoryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}

	return &store, nil
}

func (s *Server) saveStore(store *MemoryStore) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.dbPath, data, 0600)
}

func (s *Server) handleMemoryAdd() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Text string   `json:"text"`
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Text == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Text cannot be empty"}}}, nil
		}

		embeddings, err := s.client.Embed(ctx, []string{args.Text})
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to generate embedding: %v", err)}}}, nil
		}
		if len(embeddings) == 0 {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "No embedding generated"}}}, nil
		}

		store, err := s.loadStore()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to load memory store: %v", err)}}}, nil
		}

		record := MemoryRecord{
			Text:      args.Text,
			Tags:      args.Tags,
			Embedding: embeddings[0],
			Timestamp: time.Now().UTC(),
		}
		store.Records = append(store.Records, record)

		if err := s.saveStore(store); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to save memory store: %v", err)}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Memory successfully added"}}}, nil
	}
}

type SearchResult struct {
	Record MemoryRecord
	Score  float64
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (s *Server) handleMemorySearch() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Query         string  `json:"query"`
			Limit         int     `json:"limit"`
			MinSimilarity float64 `json:"min_similarity"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Query == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Query cannot be empty"}}}, nil
		}
		if args.Limit <= 0 {
			args.Limit = 5
		}

		embeddings, err := s.client.Embed(ctx, []string{args.Query})
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to generate embedding: %v", err)}}}, nil
		}
		if len(embeddings) == 0 {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "No embedding generated"}}}, nil
		}
		queryEmb := embeddings[0]

		store, err := s.loadStore()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to load memory store: %v", err)}}}, nil
		}

		var results []SearchResult
		for _, record := range store.Records {
			score := cosineSimilarity(queryEmb, record.Embedding)
			if score >= args.MinSimilarity {
				results = append(results, SearchResult{
					Record: record,
					Score:  score,
				})
			}
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		if len(results) > args.Limit {
			results = results[:args.Limit]
		}

		type responseRecord struct {
			Text      string    `json:"text"`
			Tags      []string  `json:"tags,omitempty"`
			Timestamp time.Time `json:"timestamp"`
			Score     float64   `json:"score"`
		}
		
		var out []responseRecord
		for _, res := range results {
			out = append(out, responseRecord{
				Text:      res.Record.Text,
				Tags:      res.Record.Tags,
				Timestamp: res.Record.Timestamp,
				Score:     res.Score,
			})
		}

		outBytes, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to format results: %v", err)}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(outBytes)}}}, nil
	}
}
