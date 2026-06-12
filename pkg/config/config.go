package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/borch-ai/powerword/pkg/telemetry"
	"github.com/spf13/viper"
)

// ServerConfig specifies how to launch an external MCP server.
type ServerConfig struct {
	Command string   `mapstructure:"command"`
	Args    []string `mapstructure:"args"`
	Env     []string `mapstructure:"env"`
}

// ModelPricing is a type alias to preserve backward compatibility.
type ModelPricing = telemetry.ModelPricing

// Config holds the application configuration.
type Config struct {
	Verbose           bool                              `mapstructure:"verbose"`
	Model             string                            `mapstructure:"model"`
	APIKeys           APIKeys                           `mapstructure:"api_keys"`
	Session           string                            `mapstructure:"session"`
	Resume            string                            `mapstructure:"resume"`
	ListSessions      bool                              `mapstructure:"list-sessions"`
	MaxLoopIterations int                               `mapstructure:"max_loop_iterations"`
	AutoConfirm       bool                              `mapstructure:"auto_confirm"`
	Headless          bool                              `mapstructure:"headless"`
	JSONOutput        bool                              `mapstructure:"json"`
	Servers           map[string]ServerConfig           `mapstructure:"servers"`
	Route             map[string]string                 `mapstructure:"route"`
	ClassifierModel   string                            `mapstructure:"classifier_model"`
	Pricing           map[string]telemetry.ModelPricing `mapstructure:"pricing"`
	CriticProvider    string                            `mapstructure:"critic_provider"`
	CriticModel       string                            `mapstructure:"critic_model"`
	CriticEndpoint    string                            `mapstructure:"critic_endpoint"`
	PlanTemplate      string                            `mapstructure:"plan_template"`
	Autonomous        bool                              `mapstructure:"autonomous"`
	Issue             string                            `mapstructure:"issue"`
	WebhookSecret     string                            `mapstructure:"webhook_secret"`
	WebhookPort       int                               `mapstructure:"webhook_port"`
	Plugins           PluginsConfig                     `mapstructure:"plugins"`
	GitRollback       bool                              `mapstructure:"git_rollback"`
	MaxCost           float64                           `mapstructure:"max_cost"`
	MaxTokens         int                               `mapstructure:"max_tokens"`
	MaxInputTokens    int                               `mapstructure:"max_input_tokens"`
	MaxOutputTokens   int                               `mapstructure:"max_output_tokens"`
	MaxCachedTokens   int                               `mapstructure:"max_cached_tokens"`
	OutputWriter      io.Writer                         `mapstructure:"-"`
}

// ImageGenConfig holds parameters for the image generator.
type ImageGenConfig struct {
	Backend                   string `mapstructure:"backend"`
	OpenAIAPIKey              string `mapstructure:"openai_api_key"`
	MidjourneyAPIURL          string `mapstructure:"midjourney_api_url"`
	MidjourneyAPIKey          string `mapstructure:"midjourney_api_key"`
	MidjourneyPollingInterval string `mapstructure:"midjourney_polling_interval"`
	MidjourneyPollingTimeout  string `mapstructure:"midjourney_polling_timeout"`
	GoogleAPIKey              string `mapstructure:"google_api_key"`
	GoogleModel               string `mapstructure:"google_model"`
}

// KDPMathConfig holds parameters for the KDP Math plugin.
type KDPMathConfig struct {
}

// SEOConfig holds parameters for the SEO plugin.
type SEOConfig struct {
	CacheTTLHours float64 `mapstructure:"cache_ttl_hours"`
	RateLimitMS   int     `mapstructure:"rate_limit_ms"`
}

// ViralConfig holds parameters for the Viral plugin.
type ViralConfig struct {
	TTSProvider  string `mapstructure:"tts_provider"`  // "openai", "elevenlabs", or "mock"
	TTSAPIKey    string `mapstructure:"tts_api_key"`   // optional override
	TTSVoiceID   string `mapstructure:"tts_voice_id"`  // voice to use
	VideoBackend string `mapstructure:"video_backend"` // "veo" or "mock"
	VideoAPIKey  string `mapstructure:"video_api_key"` // optional override
	FFmpegPath   string `mapstructure:"ffmpeg_path"`   // path to ffmpeg executable
}

// PluginsConfig holds configurations for individual plugins.
type PluginsConfig struct {
	ImageGen ImageGenConfig `mapstructure:"imagegen"`
	KDPMath  KDPMathConfig  `mapstructure:"kdp_math"`
	SEO      SEOConfig      `mapstructure:"seo"`
	Viral    ViralConfig    `mapstructure:"viral"`
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
	v.SetDefault("plan_template", "")
	v.SetDefault("auto_confirm", false)
	v.SetDefault("webhook_port", 8080)
	v.SetDefault("git_rollback", false)
	v.SetDefault("critic_provider", "gemini")
	v.SetDefault("critic_model", "gemini-1.5-flash")
	v.SetDefault("max_cost", 2.0)
	v.SetDefault("max_tokens", 1000000)
	v.SetDefault("max_input_tokens", 1000000)
	v.SetDefault("max_output_tokens", 100000)
	v.SetDefault("max_cached_tokens", 0)
	v.SetDefault("plugins.imagegen.backend", "openai")
	v.SetDefault("plugins.imagegen.midjourney_polling_interval", "5s")
	v.SetDefault("plugins.imagegen.midjourney_polling_timeout", "5m")
	v.SetDefault("plugins.imagegen.google_model", "imagen-3.0-generate-002")
	v.SetDefault("plugins.seo.cache_ttl_hours", 4.0)
	v.SetDefault("plugins.seo.rate_limit_ms", 500)
	v.SetDefault("plugins.viral.tts_provider", "openai")
	v.SetDefault("plugins.viral.tts_voice_id", "alloy")
	v.SetDefault("plugins.viral.video_backend", "mock")
	v.SetDefault("plugins.viral.ffmpeg_path", "ffmpeg")

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
	bindEnv(v, "resume", "POWERWORD_RESUME")
	bindEnv(v, "headless", "POWERWORD_HEADLESS")
	bindEnv(v, "json", "POWERWORD_JSON")
	bindEnv(v, "route", "POWERWORD_ROUTE")
	bindEnv(v, "classifier_model", "POWERWORD_CLASSIFIER_MODEL")
	bindEnv(v, "critic_provider", "POWERWORD_CRITIC_PROVIDER")
	bindEnv(v, "critic_model", "POWERWORD_CRITIC_MODEL")
	bindEnv(v, "critic_endpoint", "POWERWORD_CRITIC_ENDPOINT")
	bindEnv(v, "plan_template", "POWERWORD_PLAN_TEMPLATE")
	bindEnv(v, "autonomous", "POWERWORD_AUTONOMOUS")
	bindEnv(v, "issue", "POWERWORD_ISSUE")
	bindEnv(v, "webhook_secret", "POWERWORD_WEBHOOK_SECRET")
	bindEnv(v, "webhook_port", "POWERWORD_WEBHOOK_PORT")
	bindEnv(v, "git_rollback", "POWERWORD_GIT_ROLLBACK")
	bindEnv(v, "max_cost", "POWERWORD_MAX_COST")
	bindEnv(v, "max_tokens", "POWERWORD_MAX_TOKENS")
	bindEnv(v, "max_input_tokens", "POWERWORD_MAX_INPUT_TOKENS")
	bindEnv(v, "max_output_tokens", "POWERWORD_MAX_OUTPUT_TOKENS")
	bindEnv(v, "max_cached_tokens", "POWERWORD_MAX_CACHED_TOKENS")
	bindEnv(v, "plugins.imagegen.backend", "POWERWORD_IMAGEGEN_BACKEND")
	bindEnv(v, "plugins.imagegen.openai_api_key", "POWERWORD_IMAGEGEN_OPENAI_API_KEY")
	bindEnv(v, "plugins.imagegen.midjourney_api_url", "POWERWORD_IMAGEGEN_MIDJOURNEY_API_URL")
	bindEnv(v, "plugins.imagegen.midjourney_api_key", "POWERWORD_IMAGEGEN_MIDJOURNEY_API_KEY")
	bindEnv(v, "plugins.imagegen.midjourney_polling_interval", "POWERWORD_IMAGEGEN_MIDJOURNEY_POLLING_INTERVAL")
	bindEnv(v, "plugins.imagegen.midjourney_polling_timeout", "POWERWORD_IMAGEGEN_MIDJOURNEY_POLLING_TIMEOUT")
	bindEnv(v, "plugins.imagegen.google_api_key", "POWERWORD_IMAGEGEN_GOOGLE_API_KEY")
	bindEnv(v, "plugins.imagegen.google_model", "POWERWORD_IMAGEGEN_GOOGLE_MODEL")
	bindEnv(v, "plugins.seo.cache_ttl_hours", "POWERWORD_SEO_CACHE_TTL_HOURS")
	bindEnv(v, "plugins.seo.rate_limit_ms", "POWERWORD_SEO_RATE_LIMIT_MS")
	bindEnv(v, "plugins.viral.tts_provider", "POWERWORD_VIRAL_TTS_PROVIDER")
	bindEnv(v, "plugins.viral.tts_api_key", "POWERWORD_VIRAL_TTS_API_KEY")
	bindEnv(v, "plugins.viral.tts_voice_id", "POWERWORD_VIRAL_TTS_VOICE_ID")
	bindEnv(v, "plugins.viral.video_backend", "POWERWORD_VIRAL_VIDEO_BACKEND")
	bindEnv(v, "plugins.viral.video_api_key", "POWERWORD_VIRAL_VIDEO_API_KEY")
	bindEnv(v, "plugins.viral.ffmpeg_path", "POWERWORD_VIRAL_FFMPEG_PATH")

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
