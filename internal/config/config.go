package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the application configuration.
type Config struct {
	Verbose bool    `mapstructure:"verbose"`
	Model   string  `mapstructure:"model"`
	APIKeys APIKeys `mapstructure:"api_keys"`
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
func LoadConfig(cfgFile string) (*Config, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		if DefaultConfigPath != "" {
			v.SetConfigFile(DefaultConfigPath)
		} else {
			return nil, errors.New("could not determine home directory for default config path")
		}
	}

	v.SetConfigType("toml")

	// Set default values
	v.SetDefault("verbose", false)
	v.SetDefault("model", "gemini-1.5-pro")

	// Read config file if it exists.
	// If a custom config file is specified, error out if it doesn't exist.
	// If default config file is specified but doesn't exist, we ignore the error (since keys might be set in environment variables).
	if err := v.ReadInConfig(); err != nil {
		if cfgFile != "" {
			return nil, fmt.Errorf("failed to read config file %s: %w", cfgFile, err)
		}
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
		}
	}

	// Environment variable overrides
	// Explicitly bind env vars to mapstructure path
	bindEnv(v, "api_keys.gemini", "POWERWORD_GEMINI_API_KEY")
	bindEnv(v, "api_keys.openai", "POWERWORD_OPENAI_API_KEY")
	bindEnv(v, "api_keys.anthropic", "POWERWORD_ANTHROPIC_API_KEY")
	bindEnv(v, "model", "POWERWORD_MODEL")
	bindEnv(v, "verbose", "POWERWORD_VERBOSE")

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
