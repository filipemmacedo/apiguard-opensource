package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"apiguard/internal/config"
)

// setupRateLimiterServer returns a test server with rate limiter enabled.
func setupRateLimiterServer(t *testing.T, requestLimit, windowSeconds int) *Server {
	t.Helper()
	server := newTestServer(t, config.Config{UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	_, err := server.management.upsertRateLimiterConfig(rateLimiterConfigRecord{
		Enabled:                   true,
		RequestLimit:              requestLimit,
		WindowSeconds:             windowSeconds,
		QuarantineDurationSeconds: 43200,
		UpdatedAt:                 time.Now().UTC(),
		UpdatedBy:                 "test",
	})
	if err != nil {
		t.Fatalf("upsertRateLimiterConfig: %v", err)
	}
	return server
}

// TestCheckRateLimit_UnderLimit verifies requests under the limit are allowed.
func TestCheckRateLimit_UnderLimit(t *testing.T) {
	t.Parallel()
	server := setupRateLimiterServer(t, 5, 60)
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		result, err := server.checkRateLimit("user-under", now)
		if err != nil {
			t.Fatalf("checkRateLimit error on request %d: %v", i+1, err)
		}
		if !result.Allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}
}

// TestCheckRateLimit_AtLimitTriggersQuarantine verifies that hitting the limit quarantines the user.
func TestCheckRateLimit_AtLimitTriggersQuarantine(t *testing.T) {
	t.Parallel()
	server := setupRateLimiterServer(t, 3, 60)
	now := time.Now().UTC()
	userID := "user-quarantine-trigger"

	// First 3 requests should be allowed (they are inserted into the counter).
	for i := 0; i < 3; i++ {
		result, err := server.checkRateLimit(userID, now)
		if err != nil {
			t.Fatalf("checkRateLimit error on request %d: %v", i+1, err)
		}
		if !result.Allowed {
			t.Fatalf("expected request %d to be allowed, got blocked", i+1)
		}
	}

	// 4th request crosses the limit (count becomes 4 > limit 3).
	result, err := server.checkRateLimit(userID, now)
	if err != nil {
		t.Fatalf("checkRateLimit error on breach request: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected 4th request to be blocked by rate limiter")
	}
	if !result.Quarantine {
		t.Fatal("expected Quarantine=true on the breaching request")
	}
	if result.RetryAfter.IsZero() {
		t.Fatal("expected non-zero RetryAfter")
	}
}

// TestCheckRateLimit_WhileQuarantined verifies that a quarantined user is blocked immediately.
func TestCheckRateLimit_WhileQuarantined(t *testing.T) {
	t.Parallel()
	server := setupRateLimiterServer(t, 1, 60)
	now := time.Now().UTC()
	userID := "user-already-quarantined"

	// Trigger quarantine: 1st is allowed, 2nd breaches.
	if _, err := server.checkRateLimit(userID, now); err != nil {
		t.Fatalf("checkRateLimit setup error: %v", err)
	}
	breach, err := server.checkRateLimit(userID, now)
	if err != nil {
		t.Fatalf("checkRateLimit breach error: %v", err)
	}
	if breach.Allowed {
		t.Fatal("expected breach to be blocked")
	}

	// Subsequent requests should also be blocked (quarantine active).
	result, err := server.checkRateLimit(userID, now)
	if err != nil {
		t.Fatalf("checkRateLimit while-quarantined error: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected quarantined user to be blocked")
	}
	if result.Quarantine {
		t.Fatal("expected Quarantine=false (already quarantined, not a new one)")
	}
}

// TestCheckRateLimit_AfterExpiry verifies that an expired quarantine allows requests through.
func TestCheckRateLimit_AfterExpiry(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, config.Config{UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	userID := "user-expired-quarantine"

	// Manually create an already-expired quarantine.
	past := time.Now().UTC().Add(-2 * time.Hour)
	_, err := server.management.createQuarantine(userQuarantineRecord{
		UserID:    userID,
		LockedAt:  past,
		ExpiresAt: past.Add(time.Second), // expired 2h ago
	})
	if err != nil {
		t.Fatalf("createQuarantine: %v", err)
	}

	// Now enable rate limiter with a generous limit so the counter won't trigger.
	_, err = server.management.upsertRateLimiterConfig(rateLimiterConfigRecord{
		Enabled:                   true,
		RequestLimit:              100,
		WindowSeconds:             60,
		QuarantineDurationSeconds: 43200,
		UpdatedAt:                 time.Now().UTC(),
		UpdatedBy:                 "test",
	})
	if err != nil {
		t.Fatalf("upsertRateLimiterConfig: %v", err)
	}

	result, err := server.checkRateLimit(userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("checkRateLimit after expiry: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected expired quarantine to allow request through")
	}
}

// TestCheckRateLimit_Disabled verifies that disabled rate limiter allows all requests.
func TestCheckRateLimit_Disabled(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, config.Config{UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	_, err := server.management.upsertRateLimiterConfig(rateLimiterConfigRecord{
		Enabled:                   false,
		RequestLimit:              1,
		WindowSeconds:             60,
		QuarantineDurationSeconds: 43200,
		UpdatedAt:                 time.Now().UTC(),
		UpdatedBy:                 "test",
	})
	if err != nil {
		t.Fatalf("upsertRateLimiterConfig: %v", err)
	}
	userID := "user-disabled"
	for i := 0; i < 10; i++ {
		result, err := server.checkRateLimit(userID, time.Now().UTC())
		if err != nil {
			t.Fatalf("checkRateLimit request %d: %v", i+1, err)
		}
		if !result.Allowed {
			t.Fatalf("expected request %d to be allowed when disabled", i+1)
		}
	}
}

// TestUnlockQuarantine_Success verifies unlock removes the active quarantine.
func TestUnlockQuarantine_Success(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, config.Config{UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	userID := "user-unlock-test"
	now := time.Now().UTC()

	_, err := server.management.createQuarantine(userQuarantineRecord{
		UserID:    userID,
		LockedAt:  now,
		ExpiresAt: now.Add(12 * time.Hour),
	})
	if err != nil {
		t.Fatalf("createQuarantine: %v", err)
	}

	if err := server.management.unlockQuarantine(userID, now, "admin"); err != nil {
		t.Fatalf("unlockQuarantine: %v", err)
	}

	_, active, err := server.management.getActiveQuarantine(userID, now)
	if err != nil {
		t.Fatalf("getActiveQuarantine after unlock: %v", err)
	}
	if active {
		t.Fatal("expected no active quarantine after unlock")
	}
}

// TestUnlockQuarantine_NotFound verifies 404-like error when no active quarantine exists.
func TestUnlockQuarantine_NotFound(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, config.Config{UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	err := server.management.unlockQuarantine("nonexistent-user", time.Now().UTC(), "admin")
	if err == nil {
		t.Fatal("expected error when unlocking non-existent quarantine")
	}
	if !strings.Contains(err.Error(), "no active quarantine") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRateLimiterConfigValidation_AdminAPI verifies bad config is rejected.
func TestRateLimiterConfigValidation_AdminAPI(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, config.Config{UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	handler := server.Handler()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"zero limit", `{"enabled":true,"request_limit":0,"window_seconds":60,"quarantine_duration_seconds":43200}`, http.StatusBadRequest},
		{"negative window", `{"enabled":true,"request_limit":10,"window_seconds":-1,"quarantine_duration_seconds":43200}`, http.StatusBadRequest},
		{"zero quarantine", `{"enabled":true,"request_limit":10,"window_seconds":60,"quarantine_duration_seconds":0}`, http.StatusBadRequest},
		{"valid config", `{"enabled":true,"request_limit":10,"window_seconds":60,"quarantine_duration_seconds":43200}`, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost,
				"/internal/admin/api-management/guardrails/rate-limiter",
				strings.NewReader(tc.body),
			)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("expected status %d, got %d: %s", tc.want, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestProxy429OnQuarantine verifies the proxy returns 429 with Retry-After for quarantined users.
func TestProxy429OnQuarantine(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := withManagedProviderBootstrap(config.Config{UpstreamTimeout: time.Second}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	// Enable rate limiter with limit=1 so the 2nd request triggers quarantine.
	_, err := server.management.upsertRateLimiterConfig(rateLimiterConfigRecord{
		Enabled:                   true,
		RequestLimit:              1,
		WindowSeconds:             60,
		QuarantineDurationSeconds: 43200,
		UpdatedAt:                 time.Now().UTC(),
		UpdatedBy:                 "test",
	})
	if err != nil {
		t.Fatalf("upsertRateLimiterConfig: %v", err)
	}

	// Create an active user key for "test-proxy-user".
	key, rawKey, err := server.createManagedUserKey("test-proxy-user", "test", nil)
	if err != nil {
		t.Fatalf("createManagedUserKey: %v", err)
	}
	_ = key

	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`
	handler := server.Handler()

	// First request: should succeed (count becomes 1, limit is 1 — allowed).
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+rawKey)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code == http.StatusTooManyRequests {
		t.Fatalf("first request should not be rate-limited, got 429")
	}

	// Second request: count becomes 2 > limit 1, triggers quarantine → 429.
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on quarantine trigger, got %d: %s", rr2.Code, rr2.Body.String())
	}
	if rr2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}

	// Third request: user is already quarantined → 429.
	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+rawKey)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 while quarantined, got %d", rr3.Code)
	}

	// Unlock quarantine and verify the user can make requests again.
	userID := key.UserID
	if err := server.management.unlockQuarantine(userID, time.Now().UTC(), "test"); err != nil {
		t.Fatalf("unlockQuarantine: %v", err)
	}

	req4 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Authorization", "Bearer "+rawKey)
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	if rr4.Code == http.StatusTooManyRequests {
		t.Fatal("expected user to be unblocked after quarantine unlock, still got 429")
	}
}
