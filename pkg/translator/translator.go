package translator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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
	ctx           context.Context
	engine        TranslationEngine
	callbacks     TranslationCallbacks
	cache         map[string]string
	mu            sync.RWMutex
	maxConcurrent int
}

// NewTranslator 创建一个新的 LocalTranslator 实例
func NewTranslator(ctx context.Context, engine TranslationEngine, callbacks TranslationCallbacks, maxConcurrent int) *LocalTranslator {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &LocalTranslator{
		ctx:           ctx,
		engine:        engine,
		callbacks:     callbacks,
		cache:         make(map[string]string),
		maxConcurrent: maxConcurrent,
	}
}

// Translate 执行翻译操作，内部调用翻译引擎
func (t *LocalTranslator) Translate(ctx context.Context, text string) (string, error) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		// 继续执行
	}

	// 调用翻译引擎
	translatedText, err := t.engine.Translate(ctx, text)
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

// TranslateFileTexts 批量翻译文本数组（有界并发，进度按完成数上报）
func (t *LocalTranslator) TranslateFileTexts(fileName string, texts []string) ([]string, error) {
	totalItems := len(texts)
	if totalItems == 0 {
		return []string{}, nil
	}

	results := make([]string, totalItems)
	workers := t.maxConcurrent
	if workers > totalItems {
		workers = totalItems
	}

	ctx, cancel := context.WithCancel(t.ctx)
	defer cancel()

	jobs := make(chan int)
	var (
		wg       sync.WaitGroup
		done     int64
		errOnce  sync.Once
		firstErr error
	)

	reportProgress := func() {
		if t.callbacks.OnProgress == nil {
			return
		}
		current := int(atomic.AddInt64(&done, 1))
		t.callbacks.OnProgress(fileName, current, totalItems)
	}

	fail := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	worker := func() {
		defer wg.Done()
		for i := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}

			text := texts[i]

			t.mu.RLock()
			cached, found := t.cache[text]
			t.mu.RUnlock()

			if found {
				results[i] = cached
				reportProgress()
				continue
			}

			translated, err := t.Translate(ctx, text)
			if err != nil {
				fail(fmt.Errorf("translation failed for item %d in %s: %w", i, fileName, err))
				return
			}

			t.mu.Lock()
			// 二次检查，避免并发下重复写入同一 key 时覆盖不一致
			if existing, ok := t.cache[text]; ok {
				translated = existing
			} else {
				t.cache[text] = translated
			}
			t.mu.Unlock()

			results[i] = translated
			reportProgress()
		}
	}

	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go worker()
	}

	for i := range texts {
		select {
		case <-ctx.Done():
			// 停止投递剩余任务
			goto waitWorkers
		case jobs <- i:
		}
	}

waitWorkers:
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
