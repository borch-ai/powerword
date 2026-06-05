package config

import (
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	model   string
	verbose bool
)

// Active holds the successfully loaded application configuration.
var Active *Config

// NewRootCmd creates and returns a new root Cobra command.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "powerword [prompt]",
		Short: "Powerword is a vendor-agnostic Agentic CLI",
		Long: `Powerword is a lightweight, extensible command-line interface designed
to bring vendor-agnostic, agentic AI capabilities directly to your terminal
using the Model Context Protocol (MCP).`,
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip config loading/validation if no arguments are provided (shows help).
			if len(args) == 0 {
				return nil
			}

			cfg, err := LoadConfig(cfgFile)
			if err != nil {
				return err
			}

			// Flag overrides take absolute precedence
			if cmd.Flags().Changed("model") {
				cfg.Model = model
			}
			if cmd.Flags().Changed("verbose") {
				cfg.Verbose = verbose
			}

			if err := cfg.Validate(); err != nil {
				return err
			}

			Active = cfg
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			prompt := args[0]
			if Active.Verbose {
				cmd.Printf("Verbose logging enabled. Model: %s\n", Active.Model)
				cmd.Printf("Received prompt: %s\n", prompt)
			}
			cmd.Printf("Processing prompt with model %s...\n", Active.Model)
			return nil
		},
	}

	// Reset flag variables for fresh command setup
	cfgFile = ""
	model = ""
	verbose = false

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.config/powerword/config.toml)")
	cmd.PersistentFlags().StringVarP(&model, "model", "m", "", "active LLM model")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	return cmd
}

// Execute executes the root command.
func Execute() error {
	return NewRootCmd().Execute()
}
