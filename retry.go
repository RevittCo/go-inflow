package inflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (c *Client) doWithRetry(ctx context.Context, method, path string, fn func(ctx context.Context) ([]byte, error)) ([]byte, error) {
	var lastErr error
	delay := c.retryBase

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		res, err := fn(ctx)
		if err == nil {
			return res, nil
		}

		if !errors.Is(err, ErrRateLimited) {
			return nil, err
		}

		lastErr = err

		if c.onRateLimit != nil {
			c.onRateLimit(RateLimitEvent{
				Path:       path,
				Method:     method,
				Attempt:    attempt + 1,
				MaxRetries: c.maxRetries,
				RetryDelay: delay,
			})
		}

		if attempt == c.maxRetries {
			break
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
		case <-time.After(delay):
			delay = time.Duration(float64(delay) * c.retryFactor)
		}
	}

	return nil, fmt.Errorf("exceeded %d retries: %w", c.maxRetries, lastErr)
}
