package config

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile         string
	model           string
	verbose         bool
	sessionID       string
	listSessions    bool
	acceptAll       bool
	headless        bool
	jsonOutput      bool
	routeMap        map[string]string
	classifierModel string
	autonomous      bool
	issueIDString   string
	gitRollback     bool
	maxCost         float64
	maxTokens       int
	maxInputTokens  int
	maxOutputTokens int
	maxCachedTokens int
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
		SilenceUsage:      true,
		SilenceErrors:     true,
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
		printFunc := cmd.Printf
		if Active.JSONOutput {
			printFunc = func(format string, a ...interface{}) { fmt.Fprintf(os.Stderr, format, a...) }
		}
		printFunc("Verbose logging enabled. Model: %s\n", Active.Model)
		if prompt != "" {
			printFunc("Received prompt: %s\n", prompt)
		}
	}
	if !Active.ListSessions {
		if Active.JSONOutput {
			fmt.Fprintf(os.Stderr, "Processing prompt with model %s...\n", Active.Model)
		} else {
			cmd.Printf("Processing prompt with model %s...\n", Active.Model)
		}
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
	if cmd.Flags().Changed("accept-all") {
		cfg.AutoConfirm = acceptAll
	}
	if cmd.Flags().Changed("headless") {
		cfg.Headless = headless
	}
	if cmd.Flags().Changed("json") {
		cfg.JSONOutput = jsonOutput
	}
	if cmd.Flags().Changed("route") {
		cfg.Route = routeMap
	}
	if cmd.Flags().Changed("classifier-model") {
		cfg.ClassifierModel = classifierModel
	}
	if cmd.Flags().Changed("autonomous") {
		cfg.Autonomous = autonomous
	}
	if cmd.Flags().Changed("issue") {
		cfg.Issue = issueIDString
	}
	if cmd.Flags().Changed("git-rollback") {
		cfg.GitRollback = gitRollback
	}
	if cmd.Flags().Changed("max-cost") {
		cfg.MaxCost = maxCost
	}
	if cmd.Flags().Changed("max-tokens") {
		cfg.MaxTokens = maxTokens
	}
	if cmd.Flags().Changed("max-input-tokens") {
		cfg.MaxInputTokens = maxInputTokens
	}
	if cmd.Flags().Changed("max-output-tokens") {
		cfg.MaxOutputTokens = maxOutputTokens
	}
	if cmd.Flags().Changed("max-cached-tokens") {
		cfg.MaxCachedTokens = maxCachedTokens
	}
}

func setupPersistentFlags(cmd *cobra.Command) {
	// Reset flag variables for fresh command setup
	cfgFile = ""
	model = ""
	verbose = false
	sessionID = ""
	listSessions = false
	acceptAll = false
	headless = false
	jsonOutput = false
	routeMap = nil
	classifierModel = ""
	autonomous = false
	issueIDString = ""
	gitRollback = false
	maxCost = 2.0
	maxTokens = 1000000
	maxInputTokens = 1000000
	maxOutputTokens = 100000
	maxCachedTokens = 0

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is powerword.toml or $HOME/.config/powerword/config.toml)")
	cmd.PersistentFlags().StringVarP(&model, "model", "m", "", "active LLM model")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	cmd.PersistentFlags().StringVar(&sessionID, "session", "", "creates or resumes a conversation with the specified ID")
	cmd.PersistentFlags().BoolVar(&listSessions, "list-sessions", false, "lists recent conversations")
	cmd.PersistentFlags().BoolVar(&acceptAll, "accept-all", false, "bypass interactive confirmation prompts for tool executions")
	cmd.PersistentFlags().BoolVar(&headless, "headless", false, "run in headless mode, disabling interactive prompts and failing on unsafe commands")
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output structured JSON to stdout and route logs to stderr")
	cmd.PersistentFlags().StringToStringVar(&routeMap, "route", nil, "comma-separated list of pattern=model rules for routing")
	cmd.PersistentFlags().StringVar(&classifierModel, "classifier-model", "", "model to use for zero-shot prompt-based routing")
	cmd.PersistentFlags().BoolVar(&autonomous, "autonomous", false, "run an autonomous repair loop")
	cmd.PersistentFlags().StringVar(&issueIDString, "issue", "", "GitHub issue ID for autonomous review/repair")
	cmd.PersistentFlags().BoolVar(&gitRollback, "git-rollback", false, "enable automated workspace rollbacks on execution loop failure")
	cmd.PersistentFlags().Float64Var(&maxCost, "max-cost", 2.0, "maximum estimated cost budget in USD")
	cmd.PersistentFlags().IntVar(&maxTokens, "max-tokens", 1000000, "maximum total tokens budget")
	cmd.PersistentFlags().IntVar(&maxInputTokens, "max-input-tokens", 1000000, "maximum input tokens budget")
	cmd.PersistentFlags().IntVar(&maxOutputTokens, "max-output-tokens", 100000, "maximum output tokens budget")
	cmd.PersistentFlags().IntVar(&maxCachedTokens, "max-cached-tokens", 0, "maximum cached tokens budget")
}

// Execute executes the root command.
func Execute() error {
	return NewRootCmd().Execute()
}
