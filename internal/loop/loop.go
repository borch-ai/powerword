package loop

import (
	"context"
	"fmt"
	"os"

	"powerword/internal/config"
	"powerword/internal/llm"
)

// newClient is a package-level variable that defaults to llm.NewClient.
// It can be overridden in unit tests to return a mock client.
var newClient = llm.NewClient

// RunLoop runs the core execution and reasoning loop.
func RunLoop(ctx context.Context, cfg *config.Config, prompt string) error {
	client, err := newClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	messages := []llm.Message{
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	chunks, err := client.Stream(ctx, messages, nil)
	if err != nil {
		return fmt.Errorf("failed to start model stream: %w", err)
	}

	width := getTerminalWidth()
	formatter := NewTerminalFormatter(os.Stdout, width)
	defer func() { _ = formatter.Flush() }()

	for chunk := range chunks {
		if chunk.Error != nil {
			return fmt.Errorf("error in stream chunk: %w", chunk.Error)
		}
		if chunk.Content != "" {
			if _, err := formatter.Write([]byte(chunk.Content)); err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}
		}
	}

	if err := formatter.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
	}

	return nil
}
