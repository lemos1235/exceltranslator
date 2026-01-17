package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	AppName    = "Excel-Translator"
	ConfigName = "config.toml"
)

// AppConfig represents the persistent application configuration.
// It combines settings for LLMService and TextExtractor.
type AppConfig struct {
	LLM       LLMConfig       `toml:"llm" json:"llm"`
	Extractor ExtractorConfig `toml:"extractor" json:"extractor"`
}

type LLMConfig struct {
	BaseURL string `toml:"base_url" json:"base_url"`
	APIKey  string `toml:"api_key" json:"api_key"`
	Model   string `toml:"model" json:"model"`
	Prompt  string `toml:"prompt" json:"prompt"`
}

type ExtractorConfig struct {
	CJKOnly bool `toml:"cjk_only" json:"cjk_only"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *AppConfig {
	return &AppConfig{
		LLM: LLMConfig{
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:  os.Getenv("DASHSCOPE_API_KEY"),
			Model:   "qwen-flash",
			Prompt:  "Translate to Simplified Chinese.Ignore if already Chinese. Keep all numbers and letters intact.",
		},
		Extractor: ExtractorConfig{
			CJKOnly: false,
		},
	}
}

// getConfigPath returns the full path to the configuration file.
// It ensures the configuration directory exists.
func getConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config dir: %w", err)
	}

	appConfigDir := filepath.Join(configDir, AppName)
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config dir: %w", err)
	}

	return filepath.Join(appConfigDir, ConfigName), nil
}

// Load reads the configuration from the config file.
// If the file doesn't exist, it returns the default configuration.
func Load() (*AppConfig, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist, return default config
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg AppConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults if fields are missing (basic approach, or just return loaded)
	// For robust app, you might want to merge with defaults.
	// Here we'll just return what we loaded.
	return &cfg, nil
}

// Save writes the configuration to the config file.
func Save(cfg *AppConfig) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 0600: read/write for user only
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
