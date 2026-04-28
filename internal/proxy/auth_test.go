package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestAuthMiddlewareValidKeyAddsTenantAndRequestID(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}

	server := newTestServer(t, cfg, slog.Default())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := TenantIDFromContext(r.Context()); got != "tenant-a" {
			t.Fatalf("unexpected tenant_id: %q", got)
		}
		if got := RequestIDFromContext(r.Context()); got == "" {
			t.Fatal("request_id missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer tenant-key")
	rec := httptest.NewRecorder()

	server.authMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestAuthMiddlewareMissingKeyReturnsUnauthorized(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}

	server := newTestServer(t, cfg, slog.Default())

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()

	server.authMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("expected next handler not to be called")
	}
}
