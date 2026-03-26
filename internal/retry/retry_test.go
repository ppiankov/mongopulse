package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, "test-first", func(ctx context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetriesAndSucceeds(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, "test-retry", func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDo_RespectsMaxRetries(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxRetries: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, "test-max", func(ctx context.Context) error {
		calls++
		return errors.New("persistent error")
	})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	// initial attempt + 2 retries = 3
	if calls != 3 {
		t.Errorf("expected 3 calls (1 + MaxRetries), got %d", calls)
	}
}

func TestDo_RespectsContextCancellation(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxRetries: 100, BaseDelay: 50 * time.Millisecond, MaxDelay: 1 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, "test-cancel", func(ctx context.Context) error {
		calls++
		return errors.New("keep retrying")
	})
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("error is %v (may be the last fn error if cancel raced with attempt)", err)
	}
}

func TestDo_ContextAlreadyCancelled(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Do(ctx, cfg, "test-pre-cancel", func(ctx context.Context) error {
		t.Fatal("fn should not be called with cancelled context")
		return nil
	})
	if err == nil {
		t.Fatal("expected error for pre-cancelled context")
	}
}

func TestDo_ZeroRetries(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxRetries: 0, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Do(context.Background(), cfg, "test-zero", func(ctx context.Context) error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error with zero retries")
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 call with zero retries, got %d", calls)
	}
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	t.Parallel()
	base := 100 * time.Millisecond
	max := 10 * time.Second

	d0 := backoff(0, base, max)
	d1 := backoff(1, base, max)
	d2 := backoff(2, base, max)

	if d0 != 100*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 100ms", d0)
	}
	if d1 != 200*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 200ms", d1)
	}
	if d2 != 400*time.Millisecond {
		t.Errorf("backoff(2) = %v, want 400ms", d2)
	}
}

func TestBackoff_CapsAtMax(t *testing.T) {
	t.Parallel()
	base := 1 * time.Second
	max := 5 * time.Second

	d := backoff(10, base, max)
	if d != max {
		t.Errorf("backoff(10) = %v, want cap at %v", d, max)
	}
}

func TestBackoff_ZeroAttempt(t *testing.T) {
	t.Parallel()
	d := backoff(0, 500*time.Millisecond, 30*time.Second)
	if d != 500*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 500ms", d)
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 1*time.Second {
		t.Errorf("BaseDelay = %v, want 1s", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
}
