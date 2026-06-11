package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"

	"github.com/borch-ai/powerword/pkg/config"
)

func assertResponse(t *testing.T, res *mcp.CallToolResult, wantError bool, wantSubstr string) {
	t.Helper()
	if res.IsError != wantError {
		t.Errorf("expected error: %v, got: %v (content: %v)", wantError, res.IsError, res.Content)
	}
	if len(res.Content) == 0 {
		t.Errorf("no content in response")
		return
	}
	var contentStr string
	if txt, ok := res.Content[0].(*mcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(res.Content[0])
	}
	if !strings.Contains(contentStr, wantSubstr) {
		t.Errorf("expected response to contain %q, got %q", wantSubstr, contentStr)
	}
}

//nolint:funlen // test function length is acceptable
func TestImageGen_MCP(t *testing.T) {
	tempDir := t.TempDir()

	// Mock DALL-E download server
	imageBytes := []byte("mock-image-bytes")
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer downloadServer.Close()

	// Mock OpenAI DALL-E 3 generation server
	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ImageResponse{
			Data: []openai.ImageResponseDataInner{
				{URL: downloadServer.URL + "/image.png"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer openaiServer.Close()

	t.Setenv("OPENAI_BASE_URL", openaiServer.URL)

	cfg := &config.Config{
		APIKeys: config.APIKeys{
			OpenAI: "test-openai-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "openai",
			},
		},
	}

	srv, err := setupServer(tempDir, cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	// Test 1: Register Style
	regRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "imagegen_register_style",
		Arguments: json.RawMessage(`{
			"style_id": "watercolors",
			"prompt_seed": "watercolor paints on textured paper, bleeding edges"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool register style failed: %v", err)
	}
	assertResponse(t, regRes, false, "Successfully registered style profile")

	// Test 2: List Styles
	listRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "imagegen_list_styles",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool list styles failed: %v", err)
	}
	assertResponse(t, listRes, false, "watercolors")
	assertResponse(t, listRes, false, "watercolor paints on textured paper")

	// Test 3: Generate Image with registered style
	genRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "imagegen_generate",
		Arguments: json.RawMessage(`{
			"prompt": "a quiet lake at dawn",
			"size": "1024x1024",
			"style_id": "watercolors"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool generate image failed: %v", err)
	}
	assertResponse(t, genRes, false, "Successfully generated image and saved to")

	// Check if file is saved correctly in generated_images
	generatedDir := filepath.Join(tempDir, "generated_images")
	entries, err := os.ReadDir(generatedDir)
	if err != nil {
		t.Fatalf("failed to read generated images directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 file in generated_images, got %d", len(entries))
	} else {
		savedPath := filepath.Join(generatedDir, entries[0].Name())
		//nolint:gosec // savedPath is constructed safely in TestImageGen_MCP
		data, err := os.ReadFile(savedPath)
		if err != nil {
			t.Fatalf("failed to read generated file: %v", err)
		}
		if string(data) != "mock-image-bytes" {
			t.Errorf("expected mock file contents 'mock-image-bytes', got %q", string(data))
		}
	}
}
