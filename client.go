package inflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const (
	DefaultBaseURL        = "https://cloudapi.inflowinventory.com/"
	DefaultAPIVersion     = "application/json;version=2025-06-24"
	DefaultTimeout        = 30 * time.Second
	DefaultRateLimit      = 15
	DefaultBurstSize      = 5
	DefaultMaxRetries     = 3
	DefaultRetryBaseDelay = 30 * time.Second
	DefaultRetryFactor    = 2.0
)

type contextKey string

const delayContextKey contextKey = "request_delay"

func WithDelay(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, delayContextKey, d)
}

func WithNoDelay(ctx context.Context) context.Context {
	d := time.Duration(0)
	return context.WithValue(ctx, delayContextKey, d)
}

type Client struct {
	httpClient   *http.Client
	baseURL      string
	apiKey       string
	limiter      *rate.Limiter
	maxRetries   int
	retryBase    time.Duration
	retryFactor  float64
	requestDelay time.Duration
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		httpClient:  &http.Client{Timeout: DefaultTimeout},
		baseURL:     DefaultBaseURL,
		apiKey:      apiKey,
		limiter:     rate.NewLimiter(rate.Every(time.Minute/time.Duration(DefaultRateLimit)), DefaultBurstSize),
		maxRetries:  DefaultMaxRetries,
		retryBase:   DefaultRetryBaseDelay,
		retryFactor: DefaultRetryFactor,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter wait: %w", err)
	}

	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", DefaultAPIVersion)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if res.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, &APIError{
			StatusCode: res.StatusCode,
			Body:       string(bodyBytes),
		}
	}

	delay := c.requestDelay
	if d, ok := ctx.Value(delayContextKey).(time.Duration); ok {
		delay = d
	}
	if delay > 0 {
		select {
		case <-ctx.Done():
			return bodyBytes, nil
		case <-time.After(delay):
		}
	}

	return bodyBytes, nil
}

func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	return c.doWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
		return c.do(ctx, http.MethodGet, path, nil)
	})
}

func (c *Client) Post(ctx context.Context, path string, body io.Reader) ([]byte, error) {
	return c.doWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
		return c.do(ctx, http.MethodPost, path, body)
	})
}

func (c *Client) Put(ctx context.Context, path string, body io.Reader) ([]byte, error) {
	return c.doWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
		return c.do(ctx, http.MethodPut, path, body)
	})
}

func (c *Client) Delete(ctx context.Context, path string) ([]byte, error) {
	return c.doWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
		return c.do(ctx, http.MethodDelete, path, nil)
	})
}
