package runner

import (
	"context"
	"exceltranslator/pkg/config"
	"exceltranslator/pkg/fileprocessor"
	"exceltranslator/pkg/llmservice"
	"exceltranslator/pkg/logger"
	"exceltranslator/pkg/textextractor"
	"exceltranslator/pkg/translator"
	"fmt"
)

// TranslationCallbacks 定义翻译流程中的回调。
type TranslationCallbacks struct {
	OnTranslated func(original, translated string)
	OnProgress   func(phase string, done, total int)
	OnError      func(stage string, err error)
	OnComplete   func(err error)
}

// RunTranslation 执行翻译流程，通过回调报告状态。
func RunTranslation(ctx context.Context, inputFile, outputFile string, cb TranslationCallbacks) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return RunTranslationWithConfig(ctx, inputFile, outputFile, cfg, cb)
}

// RunTranslationWithConfig 执行翻译流程，使用传入的配置。
func RunTranslationWithConfig(ctx context.Context, inputFile, outputFile string, cfg *config.AppConfig, cb TranslationCallbacks) error {
	// Initialize logger
	logInstance := logger.NewLogger(100) // Max 100 lines for in-memory log

	// Initialize LLM service
	llmCfg := llmservice.LLMServiceConfig{
		BaseURL: cfg.LLM.BaseURL,
		APIKey:  cfg.LLM.APIKey,
		Model:   cfg.LLM.Model,
		Prompt:  cfg.LLM.Prompt,
	}
	llmService := llmservice.NewLLMService(llmCfg, logInstance)

	// Create LocalTranslator with context, engine, and callbacks
	translatorCallbacks := translator.TranslationCallbacks{
		OnTranslated: cb.OnTranslated,
		OnProgress:   cb.OnProgress,
		OnError:      cb.OnError,
		OnComplete:   cb.OnComplete,
	}
	trans := translator.NewTranslator(ctx, llmService, translatorCallbacks)

	// Initialize File Processor
	fp := fileprocessor.NewFileProcessorWithLogger(logInstance)
	fp.SetExtractorConfig(textextractor.ExtractorConfig{CJKOnly: cfg.Extractor.CJKOnly})

	// Process file using the LocalTranslator
	processingErr := fp.ProcessFile(inputFile, outputFile, trans)
	if processingErr != nil {
		logInstance.Errorf("File processing failed: %v", processingErr)
		cb.OnError("fileprocessor", fmt.Errorf("file processing failed: %w", processingErr))
		cb.OnComplete(processingErr)
		return processingErr
	}

	logInstance.Infof("File processing completed successfully.")
	cb.OnComplete(nil) // Final progress
	return nil
}
