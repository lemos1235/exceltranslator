package translator

import (
	"context"
	"fmt"
)

// TranslationEngine 定义翻译引擎接口，用于将原文转换成翻译结果
type TranslationEngine interface {
	// Translate 翻译给定的文本
	// ctx: 用于控制翻译过程（超时、取消等）
	// text: 待翻译的文本
	// 返回: 翻译后的文本和可能的错误
	Translate(ctx context.Context, text string) (string, error)
}

// Translator 定义翻译器接口，供 FileProcessor 使用
type Translator interface {
	// TranslateFileTexts 批量翻译文本数组
	TranslateFileTexts(fileName string, texts []string) ([]string, error)
}

// TranslationCallbacks 定义翻译流程中的回调
type TranslationCallbacks struct {
	OnTranslated func(original, translated string)
	OnProgress   func(phase string, done, total int)
	OnError      func(stage string, err error)
	OnComplete   func(err error)
}

// LocalTranslator 封装翻译引擎和上下文，负责执行翻译操作
type LocalTranslator struct {
	ctx       context.Context
	engine    TranslationEngine
	callbacks TranslationCallbacks
}

// NewTranslator 创建一个新的 LocalTranslator 实例
func NewTranslator(ctx context.Context, engine TranslationEngine, callbacks TranslationCallbacks) *LocalTranslator {
	return &LocalTranslator{
		ctx:       ctx,
		engine:    engine,
		callbacks: callbacks,
	}
}

// Translate 执行翻译操作，内部调用翻译引擎
func (t *LocalTranslator) Translate(text string) (string, error) {
	// 检查上下文是否已取消
	select {
	case <-t.ctx.Done():
		return "", t.ctx.Err()
	default:
		// 继续执行
	}

	// 调用翻译引擎
	translatedText, err := t.engine.Translate(t.ctx, text)
	if err != nil {
		if t.callbacks.OnError != nil {
			t.callbacks.OnError("translation_engine", fmt.Errorf("translation failed for text '%s': %w", text, err))
		}
		return "", err
	}

	// 只有在实际翻译发生时才触发回调
	if translatedText != text && t.callbacks.OnTranslated != nil {
		t.callbacks.OnTranslated(text, translatedText)
	}

	return translatedText, nil
}

// TranslateFileTexts 批量翻译文本数组
func (t *LocalTranslator) TranslateFileTexts(fileName string, texts []string) ([]string, error) {
	translations := make([]string, 0, len(texts))
	totalItems := len(texts)

	for i, text := range texts {
		// 翻译单个文本项
		translated, err := t.Translate(text)
		if err != nil {
			return nil, fmt.Errorf("translation failed for item %d in %s: %w", i, fileName, err)
		}
		translations = append(translations, translated)

		// 报告进度
		if t.callbacks.OnProgress != nil {
			t.callbacks.OnProgress(fileName, i+1, totalItems)
		}
	}

	return translations, nil
}
