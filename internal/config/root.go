package config

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	cfgFile      string
	model        string
	verbose      bool
	sessionID    string
	listSessions bool
)

// Active holds the successfully loaded application configuration.
var Active *Config

// Runner executes the execution loop. It must be registered by the main package to avoid import cycles.
var Runner func(ctx context.Context, cfg *Config, prompt string) error

// NewRootCmd creates and returns a new root Cobra command.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "powerword [prompt]",
		Short: "Powerword is a vendor-agnostic Agentic CLI",
		Long: `Powerword is a lightweight, extensible command-line interface designed
to bring vendor-agnostic, agentic AI capabilities directly to your terminal
using the Model Context Protocol (MCP).`,
		Version:           Version,
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: persistentPreRunE,
		RunE:              runE,
	}

	setupPersistentFlags(cmd)
	return cmd
}

func persistentPreRunE(cmd *cobra.Command, args []string) error {
	// Skip config loading/validation if just running the root command without args (shows help) unless listing sessions.
	if cmd.Name() == "powerword" && len(args) == 0 && !listSessions {
		return nil
	}

	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	applyFlagOverrides(cmd, cfg)

	if !cfg.ListSessions {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}

	Active = cfg
	return nil
}

func runE(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && (Active == nil || !Active.ListSessions) {
		return cmd.Help()
	}
	prompt := ""
	if len(args) > 0 {
		prompt = args[0]
	}
	if Active.Verbose {
		cmd.Printf("Verbose logging enabled. Model: %s\n", Active.Model)
		if prompt != "" {
			cmd.Printf("Received prompt: %s\n", prompt)
		}
	}
	if !Active.ListSessions {
		cmd.Printf("Processing prompt with model %s...\n", Active.Model)
	}
	if Runner == nil {
		return fmt.Errorf("no execution runner registered")
	}
	return Runner(cmd.Context(), Active, prompt)
}

func applyFlagOverrides(cmd *cobra.Command, cfg *Config) {
	if cmd.Flags().Changed("model") {
		cfg.Model = model
	}
	if cmd.Flags().Changed("verbose") {
		cfg.Verbose = verbose
	}
	if cmd.Flags().Changed("session") {
		cfg.Session = sessionID
	}
	if cmd.Flags().Changed("list-sessions") {
		cfg.ListSessions = listSessions
	}
}

func setupPersistentFlags(cmd *cobra.Command) {
	// Reset flag variables for fresh command setup
	cfgFile = ""
	model = ""
	verbose = false
	sessionID = ""
	listSessions = false

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is powerword.toml or $HOME/.config/powerword/config.toml)")
	cmd.PersistentFlags().StringVarP(&model, "model", "m", "", "active LLM model")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	cmd.PersistentFlags().StringVar(&sessionID, "session", "", "creates or resumes a conversation with the specified ID")
	cmd.PersistentFlags().BoolVar(&listSessions, "list-sessions", false, "lists recent conversations")
}

// Execute executes the root command.
func Execute() error {
	return NewRootCmd().Execute()
}
