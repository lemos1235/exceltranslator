package translator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockEngine struct {
	delay       time.Duration
	failAt      map[string]error
	calls       atomic.Int64
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
}

func (m *mockEngine) Translate(ctx context.Context, text string) (string, error) {
	m.calls.Add(1)
	cur := m.inFlight.Add(1)
	for {
		old := m.maxInFlight.Load()
		if cur <= old || m.maxInFlight.CompareAndSwap(old, cur) {
			break
		}
	}
	defer m.inFlight.Add(-1)

	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(m.delay):
		}
	}

	if err, ok := m.failAt[text]; ok {
		return "", err
	}
	return "T:" + text, nil
}

func TestTranslateFileTexts_PreservesOrderAndProgress(t *testing.T) {
	engine := &mockEngine{delay: 20 * time.Millisecond}
	var (
		mu       sync.Mutex
		progress []int
		totals   []int
	)

	tr := NewTranslator(context.Background(), engine, TranslationCallbacks{
		OnProgress: func(phase string, done, total int) {
			mu.Lock()
			defer mu.Unlock()
			progress = append(progress, done)
			totals = append(totals, total)
		},
	}, 4)

	texts := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	got, err := tr.TranslateFileTexts("sheet1.xml", texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != len(texts) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(texts))
	}
	for i, text := range texts {
		want := "T:" + text
		if got[i] != want {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(progress) != len(texts) {
		t.Fatalf("progress callbacks=%d, want %d", len(progress), len(texts))
	}
	seen := make(map[int]bool, len(progress))
	for i, d := range progress {
		if d < 1 || d > len(texts) {
			t.Errorf("progress[%d]=%d out of range", i, d)
		}
		if seen[d] {
			t.Errorf("duplicate done value %d", d)
		}
		seen[d] = true
		if totals[i] != len(texts) {
			t.Errorf("total[%d]=%d, want %d", i, totals[i], len(texts))
		}
	}
	for i := 1; i <= len(texts); i++ {
		if !seen[i] {
			t.Errorf("missing done value %d", i)
		}
	}

	if engine.maxInFlight.Load() < 2 {
		t.Fatalf("expected concurrent calls, maxInFlight=%d", engine.maxInFlight.Load())
	}
}

func TestTranslateFileTexts_UsesCache(t *testing.T) {
	engine := &mockEngine{}
	tr := NewTranslator(context.Background(), engine, TranslationCallbacks{}, 4)

	// 先填充缓存
	if _, err := tr.TranslateFileTexts("doc.xml", []string{"x", "y"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if engine.calls.Load() != 2 {
		t.Fatalf("seed engine calls=%d, want 2", engine.calls.Load())
	}

	texts := []string{"x", "y", "x", "y", "x"}
	got, err := tr.TranslateFileTexts("doc.xml", texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, text := range texts {
		if got[i] != "T:"+text {
			t.Errorf("got[%d]=%q", i, got[i])
		}
	}
	// 第二次应全部命中缓存
	if engine.calls.Load() != 2 {
		t.Fatalf("engine calls=%d, want 2 (cache hits)", engine.calls.Load())
	}
}

func TestTranslateFileTexts_FirstErrorStops(t *testing.T) {
	engine := &mockEngine{
		delay:  10 * time.Millisecond,
		failAt: map[string]error{"bad": errors.New("boom")},
	}
	tr := NewTranslator(context.Background(), engine, TranslationCallbacks{}, 4)

	texts := make([]string, 20)
	for i := range texts {
		texts[i] = fmt.Sprintf("t%d", i)
	}
	texts[3] = "bad"

	_, err := tr.TranslateFileTexts("err.xml", texts)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTranslateFileTexts_Cancel(t *testing.T) {
	engine := &mockEngine{delay: 200 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	tr := NewTranslator(ctx, engine, TranslationCallbacks{}, 4)

	texts := []string{"a", "b", "c", "d", "e", "f"}
	errCh := make(chan error, 1)
	go func() {
		_, err := tr.TranslateFileTexts("cancel.xml", texts)
		errCh <- err
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancel")
	}
}

func TestNewTranslator_InvalidMaxConcurrent(t *testing.T) {
	tr := NewTranslator(context.Background(), &mockEngine{}, TranslationCallbacks{}, 0)
	if tr.maxConcurrent != 1 {
		t.Fatalf("maxConcurrent=%d, want 1", tr.maxConcurrent)
	}
}
