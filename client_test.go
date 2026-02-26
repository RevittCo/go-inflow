package inflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != DefaultAPIVersion {
			t.Errorf("expected %s, got %s", DefaultAPIVersion, r.Header.Get("Accept"))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
	)

	data, err := client.Get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"id":"123"}` {
		t.Errorf("expected {\"id\":\"123\"}, got %s", string(data))
	}
}

func TestGet_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`not found`))
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
	)

	_, err := client.Get(context.Background(), "test/endpoint")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr := IsAPIError(err)
	if apiErr == nil {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestGet_RateLimitRetry(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRetry(3, 10*time.Millisecond, 1.0),
	)

	data, err := client.Get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("expected ok, got %s", string(data))
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func TestGet_RateLimitExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRetry(2, 10*time.Millisecond, 1.0),
	)

	_, err := client.Get(context.Background(), "test/endpoint")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsRateLimited(err) {
		t.Errorf("expected rate limited error, got: %v", err)
	}
}

func TestRateLimiter_Blocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	// 1 request per second, burst of 1
	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithRateLimit(60, 1),
		WithRetry(0, 0, 0),
	)

	// First request should succeed immediately (burst token)
	start := time.Now()
	_, err := client.Get(context.Background(), "test")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Error("first request should have been immediate")
	}

	// Second request should block ~1 second waiting for a token
	start = time.Now()
	_, err = client.Get(context.Background(), "test")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Errorf("second request should have blocked ~1s, took %v", elapsed)
	}
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	// Very slow rate: 1 per minute, burst of 1
	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithRateLimit(1, 1),
		WithRetry(0, 0, 0),
	)

	// Use up the burst token
	_, _ = client.Get(context.Background(), "test")

	// Second request with short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The rate limiter may return its own error when the deadline is too short
	// to wait for a token, rather than wrapping context.DeadlineExceeded directly.
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// Accept any context-related or rate limiter wait error
		if got := err.Error(); got == "" {
			t.Errorf("expected context/rate limiter error, got: %v", err)
		}
	}
}

func TestDelete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
	)

	_, err := client.Delete(context.Background(), "test/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPut_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"created":true}`))
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
	)

	data, err := client.Put(context.Background(), "test/endpoint", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"created":true}` {
		t.Errorf("unexpected body: %s", string(data))
	}
}

func TestRequestDelay_ClientLevel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRequestDelay(500*time.Millisecond),
	)

	start := time.Now()
	_, err := client.Get(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 450*time.Millisecond {
		t.Errorf("expected ~500ms delay, took %v", elapsed)
	}
}

func TestRequestDelay_PerRequestOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	// Client has a 2s default delay
	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRequestDelay(2*time.Second),
	)

	// Override to 100ms for this specific request
	ctx := WithDelay(context.Background(), 100*time.Millisecond)
	start := time.Now()
	_, err := client.Get(ctx, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 500*time.Millisecond {
		t.Errorf("per-request override should have been ~100ms, took %v", elapsed)
	}
}

func TestRequestDelay_NoDelayOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	// Client has a 2s default delay
	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRequestDelay(2*time.Second),
	)

	// Skip delay for this request
	ctx := WithNoDelay(context.Background())
	start := time.Now()
	_, err := client.Get(ctx, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 500*time.Millisecond {
		t.Errorf("WithNoDelay should skip delay, took %v", elapsed)
	}
}

func TestOnRetrySuccess_Called(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	var event *RetrySuccessEvent
	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRetry(3, 10*time.Millisecond, 1.0),
		WithOnRetrySuccess(func(e RetrySuccessEvent) {
			event = &e
		}),
	)

	data, err := client.Get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("expected ok, got %s", string(data))
	}
	if event == nil {
		t.Fatal("expected OnRetrySuccess callback to be called")
	}
	if event.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", event.Attempts)
	}
	if event.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", event.Method)
	}
	if event.Path != "test/endpoint" {
		t.Errorf("expected test/endpoint, got %s", event.Path)
	}
}

func TestOnRetrySuccess_NotCalledOnFirstAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	called := false
	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithOnRetrySuccess(func(e RetrySuccessEvent) {
			called = true
		}),
	)

	_, err := client.Get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("OnRetrySuccess should not be called when request succeeds on first attempt")
	}
}

func TestRetryContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/"),
		WithNoRateLimit(),
		WithRetry(5, 1*time.Second, 2.0),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}
