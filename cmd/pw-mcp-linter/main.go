package main

import (
	"context"
	"fmt"
	"os"

	"github.com/borch-ai/powerword/internal/mcp/linter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv := linter.SetupServer()
	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}
