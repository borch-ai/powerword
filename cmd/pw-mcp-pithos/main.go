package main

import (
	"context"
	"fmt"
	"os"

	"github.com/borch-ai/powerword/internal/mcp/pithos"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pw-mcp-pithos fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv, err := pithos.SetupServer()
	if err != nil {
		return fmt.Errorf("failed to setup server: %w", err)
	}

	transport := &mcp.StdioTransport{}
	if err := srv.Run(context.Background(), transport); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
