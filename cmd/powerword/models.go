package main

import (
	"fmt"

	"powerword/internal/config"
	"powerword/internal/llm"

	"github.com/spf13/cobra"
)

func newModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available models for the currently configured provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Active
			if cfg == nil {
				return fmt.Errorf("configuration not loaded")
			}

			client, err := llm.NewClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to initialize LLM client: %w", err)
			}

			cmd.Printf("Fetching available models...\n")
			models, err := client.ListModels(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list models: %w", err)
			}

			cmd.Printf("\nAvailable Models:\n")
			for _, m := range models {
				cmd.Printf(" - %s\n", m)
			}
			return nil
		},
	}
}
