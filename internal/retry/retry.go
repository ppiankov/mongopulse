package retry

import (
	"context"
	"log"
	"math"
	"time"
)

type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxRetries: 5,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

func Do(ctx context.Context, cfg Config, label string, fn func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			if attempt > 0 {
				log.Printf("[retry] %s: succeeded after %d retries", label, attempt)
			}
			return nil
		}

		if attempt == cfg.MaxRetries {
			break
		}

		delay := backoff(attempt, cfg.BaseDelay, cfg.MaxDelay)
		log.Printf("[retry] %s: attempt %d failed (%v), retrying in %s", label, attempt+1, lastErr, delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func backoff(attempt int, base, max time.Duration) time.Duration {
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if d > max {
		d = max
	}
	return d
}
