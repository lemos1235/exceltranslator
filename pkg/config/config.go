package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	AppName  = "Excel-Translator"
	FileName = "config.toml"
)

const DefaultMaxConcurrentRequests = 4

// AppConfig represents the persistent application configuration.
// It combines settings for LLMService and translation behavior.
type AppConfig struct {
	LLM         LLMConfig         `toml:"llm" json:"llm"`
	Translation TranslationConfig `toml:"translation" json:"translation"`
	Log         LogConfig         `toml:"log" json:"log"`
	Window      WindowConfig      `toml:"window" json:"window"`
}

type LLMConfig struct {
	BaseURL string `toml:"base_url" json:"base_url"`
	APIKey  string `toml:"api_key" json:"api_key"`
	Model   string `toml:"model" json:"model"`
	Prompt  string `toml:"prompt" json:"prompt"`
}

type TranslationConfig struct {
	CJKOnly               bool `toml:"cjk_only" json:"cjk_only"`
	MaxConcurrentRequests int  `toml:"max_concurrent_requests" json:"max_concurrent_requests"`
}

type LogConfig struct {
	Level    string `toml:"level" json:"level"`
	Disabled bool   `toml:"disabled" json:"disabled"`
}

type WindowConfig struct {
	Width  int `toml:"width" json:"width"`
	Height int `toml:"height" json:"height"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *AppConfig {
	return &AppConfig{
		LLM: LLMConfig{
			BaseURL: "https://api.deepseek.com",
			APIKey:  "",
			Model:   "deepseek-v4-flash",
			Prompt:  "你是专业翻译引擎。请将待翻译文本翻译为简体中文。保留原文中的数字、字母、占位符、标点和换行。若原文已经是中文则原样返回。",
		},
		Translation: TranslationConfig{
			CJKOnly:               false,
			MaxConcurrentRequests: DefaultMaxConcurrentRequests,
		},
		Log: LogConfig{
			Level:    "INFO",
			Disabled: false,
		},
		Window: WindowConfig{
			Width:  0,
			Height: 0,
		},
	}
}

// Normalize fills invalid or missing values with safe defaults.
func (cfg *AppConfig) Normalize() {
	if cfg.Translation.MaxConcurrentRequests <= 0 {
		cfg.Translation.MaxConcurrentRequests = DefaultMaxConcurrentRequests
	}
	// base_url 不在这里收敛：那是 SDK 的路径拼接约定，属于 llmservice 的职责，
	// 由 NewLLMService 在用的时候归一。config 若反过来 import llmservice，
	// llmservice 将来读配置就会形成 import cycle。
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

	return filepath.Join(appConfigDir, FileName), nil
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

	cfg.Normalize()
	return &cfg, nil
}

// Save writes the configuration to the config file.
func Save(cfg *AppConfig) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	cfg.Normalize()

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
