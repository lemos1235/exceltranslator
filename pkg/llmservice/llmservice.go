package llmservice

import (
	"context"
	"exceltranslator/pkg/logger" // Import the logger package
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// LLMServiceConfig holds the configuration for the LLM service.
type LLMServiceConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string // Base prompt for translation
}

// LLMService provides translation capabilities using an OpenAI-compatible API.
type LLMService struct {
	config LLMServiceConfig
	client *openai.Client
	logger *logger.Logger // Logger instance
}

// NewLLMService creates a new LLMService instance.
func NewLLMService(config LLMServiceConfig, log *logger.Logger) *LLMService {
	baseURL := config.BaseURL

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(config.APIKey),
		option.WithRequestTimeout(60*time.Second),
		option.WithMaxRetries(3),
	)

	return &LLMService{
		config: config,
		client: &client,
		logger: log, // Assign the logger
	}
}

// Translate translates the given text using the configured LLM with retries.
func (s *LLMService) Translate(ctx context.Context, text string) (string, error) {
	trimmed := strings.TrimSpace(text)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(s.config.Prompt),
			openai.UserMessage(trimmed),
		},
		Model:       s.config.Model,
		Temperature: openai.Float(0),
		Metadata:    map[string]string{"enable_thinking": "false"},
	}

	if strings.HasPrefix(s.config.Model, "qwen-mt") {
		params = openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(s.config.Prompt + "\n\n" + trimmed),
			},
			Model:       s.config.Model,
			Temperature: openai.Float(0),
			Metadata:    map[string]string{"enable_thinking": "false"},
		}
	}

	chatCompletion, err := s.client.Chat.Completions.New(ctx, params)
	if err != nil {
		s.logger.Errorf("Failed to create chat completion: %v", err)
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(chatCompletion.Choices) == 0 {
		s.logger.Warnf("No translation choices found in LLM response.")
		return "", fmt.Errorf("no translation choices found in response")
	}

	return chatCompletion.Choices[0].Message.Content, nil
}
