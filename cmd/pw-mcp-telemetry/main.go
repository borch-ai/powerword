package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/mcp/telemetry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv, err := telemetry.SetupServer()
	if err != nil {
		return fmt.Errorf("failed to setup telemetry server: %w", err)
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}
