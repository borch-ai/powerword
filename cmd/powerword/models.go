package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"powerword/internal/config"
	"powerword/internal/llm"
)

func newModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available models for the currently configured provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			if config.Active == nil {
				// Initialize config if not already done by persistent pre-run
				// But root.go's PersistentPreRunE already sets config.Active.
				// Wait, if no args are provided to root, PersistentPreRunE skips config loading.
				// So we must ensure config is loaded. Actually, root.go skips if len(args) == 0, but for subcommands it might also skip if args are passed to the subcommand, not root.
				// Let's check root.go's PersistentPreRunE.
			}
			
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
