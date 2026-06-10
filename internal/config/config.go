package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ServerConfig specifies how to launch an external MCP server.
type ServerConfig struct {
	Command string   `mapstructure:"command"`
	Args    []string `mapstructure:"args"`
	Env     []string `mapstructure:"env"`
}

// ModelPricing specifies the cost per 1M tokens.
type ModelPricing struct {
	Input  float64 `mapstructure:"input"`
	Output float64 `mapstructure:"output"`
	Cached float64 `mapstructure:"cached"`
}

// Config holds the application configuration.
type Config struct {
	Verbose           bool                    `mapstructure:"verbose"`
	Model             string                  `mapstructure:"model"`
	APIKeys           APIKeys                 `mapstructure:"api_keys"`
	Session           string                  `mapstructure:"session"`
	ListSessions      bool                    `mapstructure:"list-sessions"`
	MaxLoopIterations int                     `mapstructure:"max_loop_iterations"`
	AutoConfirm       bool                    `mapstructure:"auto_confirm"`
	Headless          bool                    `mapstructure:"headless"`
	JSONOutput        bool                    `mapstructure:"json"`
	Servers           map[string]ServerConfig `mapstructure:"servers"`
	Route             map[string]string       `mapstructure:"route"`
	ClassifierModel   string                  `mapstructure:"classifier_model"`
	Pricing           map[string]ModelPricing `mapstructure:"pricing"`
	CriticProvider    string                  `mapstructure:"critic_provider"`
	CriticModel       string                  `mapstructure:"critic_model"`
	CriticEndpoint    string                  `mapstructure:"critic_endpoint"`
	Autonomous        bool                    `mapstructure:"autonomous"`
	Issue             string                  `mapstructure:"issue"`
	WebhookSecret     string                  `mapstructure:"webhook_secret"`
	WebhookPort       int                     `mapstructure:"webhook_port"`
	Daemon            bool                    `mapstructure:"daemon"`
	FirebaseProject   string                  `mapstructure:"firebase_project"`
}

// APIKeys maps the model providers to their API keys.
type APIKeys struct {
	Gemini    string `mapstructure:"gemini"`
	OpenAI    string `mapstructure:"openai"`
	Anthropic string `mapstructure:"anthropic"`
}

// DefaultConfigPath holds the standard path to the configuration file.
var DefaultConfigPath string

func init() {
	home, err := os.UserHomeDir()
	if err == nil {
		DefaultConfigPath = filepath.Join(home, ".config", "powerword", "config.toml")
	}
}

// loadDotEnv reads the local .env file if it exists and pushes the keys into the process environment
// so that BindEnv can pick them up. It only loads POWERWORD_ prefixed variables and does not overwrite
// existing environment variables.
func loadDotEnv() error {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read .env file: %w", err)
	}

	for _, key := range v.AllKeys() {
		upperKey := strings.ToUpper(key)
		if !strings.HasPrefix(upperKey, "POWERWORD_") {
			continue
		}
		// Never overwrite already present environment variables
		if os.Getenv(upperKey) != "" {
			continue
		}

		val := v.GetString(key)
		if err := os.Setenv(upperKey, val); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %w", upperKey, err)
		}
	}

	return nil
}

// bindEnv is a helper to bind environment variables to Viper. It panics if the binding fails,
// which is an unrecoverable setup error since the key and environment variable name are statically defined.
func bindEnv(v *viper.Viper, input ...string) {
	if err := v.BindEnv(input...); err != nil {
		panic(fmt.Errorf("failed to bind env variable: %w", err))
	}
}

// LoadConfig loads the configuration using Viper.
//
//nolint:funlen // Config loading is inherently lengthy
func LoadConfig(cfgFile string) (*Config, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	v := viper.New()

	var configFilesToTry []string
	if cfgFile != "" {
		configFilesToTry = []string{cfgFile}
	} else {
		configFilesToTry = []string{"powerword.toml"}
		if DefaultConfigPath != "" {
			configFilesToTry = append(configFilesToTry, DefaultConfigPath)
		} else {
			return nil, errors.New("could not determine home directory for default config path")
		}
	}

	v.SetConfigType("toml")

	// Set default values
	v.SetDefault("verbose", false)
	v.SetDefault("model", "gemini-1.5-pro")
	v.SetDefault("max_loop_iterations", 10)
	v.SetDefault("auto_confirm", false)
	v.SetDefault("webhook_port", 8080)

	// Read config files in order
	var readErr error
	for _, file := range configFilesToTry {
		v.SetConfigFile(file)
		err := v.ReadInConfig()
		if err == nil {
			readErr = nil
			break
		}
		if cfgFile != "" {
			readErr = fmt.Errorf("failed to read config file %s: %w", cfgFile, err)
			break
		}
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && !os.IsNotExist(err) {
			readErr = fmt.Errorf("failed to parse config file %s: %w", file, err)
			break
		}
		readErr = err
	}

	if readErr != nil {
		if cfgFile != "" {
			return nil, readErr
		}
		if _, ok := readErr.(viper.ConfigFileNotFoundError); !ok && !os.IsNotExist(readErr) {
			return nil, readErr
		}
	}

	// Environment variable overrides
	// Explicitly bind env vars to mapstructure path
	bindEnv(v, "api_keys.gemini", "POWERWORD_GEMINI_API_KEY")
	bindEnv(v, "api_keys.openai", "POWERWORD_OPENAI_API_KEY")
	bindEnv(v, "api_keys.anthropic", "POWERWORD_ANTHROPIC_API_KEY")
	bindEnv(v, "model", "POWERWORD_MODEL")
	bindEnv(v, "verbose", "POWERWORD_VERBOSE")
	bindEnv(v, "max_loop_iterations", "POWERWORD_MAX_LOOP_ITERATIONS")
	bindEnv(v, "auto_confirm", "POWERWORD_AUTO_CONFIRM")
	bindEnv(v, "headless", "POWERWORD_HEADLESS")
	bindEnv(v, "json", "POWERWORD_JSON")
	bindEnv(v, "route", "POWERWORD_ROUTE")
	bindEnv(v, "classifier_model", "POWERWORD_CLASSIFIER_MODEL")
	bindEnv(v, "critic_provider", "POWERWORD_CRITIC_PROVIDER")
	bindEnv(v, "critic_model", "POWERWORD_CRITIC_MODEL")
	bindEnv(v, "critic_endpoint", "POWERWORD_CRITIC_ENDPOINT")
	bindEnv(v, "autonomous", "POWERWORD_AUTONOMOUS")
	bindEnv(v, "issue", "POWERWORD_ISSUE")
	bindEnv(v, "webhook_secret", "POWERWORD_WEBHOOK_SECRET")
	bindEnv(v, "webhook_port", "POWERWORD_WEBHOOK_PORT")
	bindEnv(v, "daemon", "POWERWORD_DAEMON")
	bindEnv(v, "firebase_project", "POWERWORD_FIREBASE_PROJECT")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// Validate validates that the configuration is correct, specifically checking
// if at least one API key is present.
func (c *Config) Validate() error {
	if c.APIKeys.Gemini == "" && c.APIKeys.OpenAI == "" && c.APIKeys.Anthropic == "" {
		return errors.New("no API keys found; at least one of Gemini, OpenAI, or Anthropic API keys must be provided via config or environment variables")
	}
	return nil
}
