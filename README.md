# go-inflow

Rate-limited HTTP client for the Inflow API. Provides token bucket rate limiting, automatic retry on 429s, and configurable per-request delays.

## Install

```
go get github.com/RevittCo/go-inflow
```

## Usage

```go
client := inflow.NewClient("your-api-key")

data, err := client.Get(ctx, "company-id/sales-orders/123")
```

### Options

```go
client := inflow.NewClient("your-api-key",
    inflow.WithRateLimit(20, 5),                          // 20 req/min, burst of 5
    inflow.WithRequestDelay(2 * time.Second),             // sleep after each request
    inflow.WithRetry(3, 30 * time.Second, 2.0),           // 3 retries, 30s base, 2x backoff
    inflow.WithBaseURL("http://localhost:9000/"),          // override API URL
    inflow.WithTimeout(10 * time.Second),                 // HTTP client timeout
)
```

| Option | Default | Description |
|--------|---------|-------------|
| `WithRateLimit(rpm, burst)` | 15 rpm, burst 5 | Token bucket rate limit |
| `WithRequestDelay(d)` | 0 (disabled) | Sleep after each successful request |
| `WithRetry(max, base, factor)` | 3 retries, 30s, 2x | Exponential backoff on 429 |
| `WithBaseURL(url)` | `https://cloudapi.inflowinventory.com/` | API base URL |
| `WithTimeout(d)` | 30s | HTTP client timeout |
| `WithHTTPClient(hc)` | default | Custom `http.Client` |
| `WithLimiter(l)` | default | Custom `rate.Limiter` |
| `WithNoRateLimit()` | - | Disable rate limiting (testing only) |

### Per-Request Delay Control

Override the client's default delay for individual requests:

```go
// Skip delay for a frontend-triggered request
ctx := inflow.WithNoDelay(ctx)
data, err := client.Get(ctx, path)

// Custom delay for a specific request
ctx := inflow.WithDelay(ctx, 500 * time.Millisecond)
data, err := client.Get(ctx, path)
```

### HTTP Methods

```go
client.Get(ctx, "company-id/sales-orders/123")
client.Post(ctx, "company-id/webhooks", body)
client.Put(ctx, "company-id/webhooks", body)
client.Delete(ctx, "company-id/webhooks/123")
```

All methods set `Authorization`, `Content-Type`, and `Accept` headers automatically.

### Error Handling

```go
data, err := client.Get(ctx, path)
if err != nil {
    if inflow.IsRateLimited(err) {
        // 429 after all retries exhausted
    }
    if apiErr := inflow.IsAPIError(err); apiErr != nil {
        // non-2xx response
        log.Error("status", apiErr.StatusCode, "body", apiErr.Body)
    }
    if errors.Is(err, context.DeadlineExceeded) {
        // context expired while waiting for rate limiter token
    }
}
```

### Rate Budget Allocation

The Inflow API allows 60 requests per minute globally. Split the budget across services:

```go
// Service A: webhook dashboard
inflow.NewClient(key, inflow.WithRateLimit(20, 5))

// Service B: cron jobs
inflow.NewClient(key, inflow.WithRateLimit(15, 3), inflow.WithRequestDelay(2 * time.Second))

// Service C: PO creation
inflow.NewClient(key, inflow.WithRateLimit(15, 5))
```
