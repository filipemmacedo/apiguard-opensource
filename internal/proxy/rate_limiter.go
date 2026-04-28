package proxy

import (
	"fmt"
	"time"
)

const (
	guardrailTypeRateLimiter       = "rate_limiter"
	rateLimiterActionQuarantine    = "quarantine"
	rateLimiterActionBlock         = "block"
	rateLimiterDefaultLimit        = 100
	rateLimiterDefaultWindowSecs   = 60
	rateLimiterDefaultQuarantineSecs = 43200 // 12 hours
)

// checkRateLimitResult describes the outcome of a rate-limit evaluation.
type checkRateLimitResult struct {
	Allowed    bool
	Quarantine bool // true when this call triggered a new quarantine
	RetryAfter time.Time
}

// checkRateLimit evaluates whether userID is allowed to make a request right now.
// It reads an active quarantine first (fast path), then counts the sliding-window,
// and triggers a new quarantine when the limit is exceeded.
// The caller must supply the current time so tests can control time.
func (s *Server) checkRateLimit(userID string, now time.Time) (checkRateLimitResult, error) {
	cfg, found, err := s.management.getRateLimiterConfig()
	if err != nil {
		return checkRateLimitResult{}, fmt.Errorf("rate limiter: load config: %w", err)
	}
	if !found || !cfg.Enabled {
		return checkRateLimitResult{Allowed: true}, nil
	}

	// Fast path: already quarantined.
	quarantine, active, err := s.management.getActiveQuarantine(userID, now)
	if err != nil {
		return checkRateLimitResult{}, fmt.Errorf("rate limiter: check quarantine: %w", err)
	}
	if active {
		return checkRateLimitResult{Allowed: false, RetryAfter: quarantine.ExpiresAt}, nil
	}

	// Prune stale counter rows for this evaluation window before counting.
	windowDur := time.Duration(cfg.WindowSeconds) * time.Second
	windowStart := now.Add(-windowDur)
	if err := s.management.deleteRequestsOlderThan(windowStart); err != nil {
		// Non-fatal: log and continue; counter may be slightly inflated.
		s.logger.Warn("rate limiter: failed to prune stale requests", "error", err)
	}

	// Record this request then count (slight overshoot is acceptable).
	if err := s.management.insertRateLimitRequest(userID, now); err != nil {
		return checkRateLimitResult{}, fmt.Errorf("rate limiter: record request: %w", err)
	}

	count, err := s.management.countRequestsInWindow(userID, windowStart)
	if err != nil {
		return checkRateLimitResult{}, fmt.Errorf("rate limiter: count requests: %w", err)
	}

	if count <= cfg.RequestLimit {
		return checkRateLimitResult{Allowed: true}, nil
	}

	// Limit exceeded — quarantine the user and wipe the counter.
	quarantineDur := time.Duration(cfg.QuarantineDurationSeconds) * time.Second
	expiresAt := now.Add(quarantineDur)
	newRecord := userQuarantineRecord{
		UserID:       userID,
		LockedAt:     now,
		ExpiresAt:    expiresAt,
		LockedReason: fmt.Sprintf("exceeded %d requests in %ds window", cfg.RequestLimit, cfg.WindowSeconds),
	}
	if _, err := s.management.createQuarantine(newRecord); err != nil {
		return checkRateLimitResult{}, fmt.Errorf("rate limiter: create quarantine: %w", err)
	}
	if err := s.management.deleteRequestsByUserID(userID); err != nil {
		s.logger.Warn("rate limiter: failed to clear counter after quarantine", "user_id", userID, "error", err)
	}

	return checkRateLimitResult{Allowed: false, Quarantine: true, RetryAfter: expiresAt}, nil
}
