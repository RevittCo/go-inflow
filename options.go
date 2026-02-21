package inflow

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func WithRateLimit(requestsPerMinute int, burst int) Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(
			rate.Every(time.Minute/time.Duration(requestsPerMinute)),
			burst,
		)
	}
}

func WithLimiter(l *rate.Limiter) Option {
	return func(c *Client) { c.limiter = l }
}

func WithRetry(maxRetries int, baseDelay time.Duration, factor float64) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.retryBase = baseDelay
		c.retryFactor = factor
	}
}

func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

func WithNoRateLimit() Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Inf, 0)
	}
}

func WithRequestDelay(d time.Duration) Option {
	return func(c *Client) { c.requestDelay = d }
}

func WithOnRateLimit(fn func(event RateLimitEvent)) Option {
	return func(c *Client) { c.onRateLimit = fn }
}
