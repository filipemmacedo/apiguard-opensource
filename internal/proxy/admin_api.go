package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	adminAPIManagementPath = "/internal/admin/api-management"
)

func adminActorFromContext(_ interface{}) string {
	return "admin"
}

func (s *Server) adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFromHeaderOrGenerate(r.Header.Get("X-Request-ID"))
		r = r.WithContext(WithRequestID(r.Context(), requestID))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAdminAPIManagement(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, adminAPIManagementPath)
	if path == "" || path == "/" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	if path == "/tenant-keys" {
		s.handleAdminUserKeys(w, r)
		return
	}
	if strings.HasPrefix(path, "/tenant-keys/") {
		s.handleAdminUserKeyByID(w, r, strings.TrimPrefix(path, "/tenant-keys/"))
		return
	}
	if path == "/providers" {
		s.handleAdminProviders(w, r)
		return
	}
	if strings.HasPrefix(path, "/providers/") {
		s.handleAdminProviderByID(w, r, strings.TrimPrefix(path, "/providers/"))
		return
	}
	if path == "/pii-policy" {
		s.handleAdminPIIPolicy(w, r)
		return
	}
	if path == "/nsfw-terms" {
		s.handleAdminNSFWTerms(w, r)
		return
	}
	if strings.HasPrefix(path, "/nsfw-terms/") {
		s.handleAdminNSFWTermByID(w, r, strings.TrimPrefix(path, "/nsfw-terms/"))
		return
	}
	if path == "/guardrails/rate-limiter" {
		s.handleAdminRateLimiterConfig(w, r)
		return
	}
	if path == "/quarantine" {
		s.handleAdminQuarantine(w, r)
		return
	}
	if strings.HasPrefix(path, "/quarantine/") {
		s.handleAdminQuarantineByUserID(w, r, strings.TrimPrefix(path, "/quarantine/"))
		return
	}

	writeJSONError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminUserKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keys, err := s.listManagedUserKeys()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load user keys")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_keys": keys})
	case http.MethodPost:
		var payload struct {
			UserID      *string `json:"user_id"`
			DisplayName string  `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		payload.DisplayName = strings.TrimSpace(payload.DisplayName)
		if payload.UserID != nil {
			writeJSONError(w, http.StatusBadRequest, "user_id is assigned by the server")
			return
		}

		keyView, rawKey, err := s.createManagedUserKey(payload.DisplayName, adminActorFromContext(r.Context()), nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to create user key")
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"user_key.create",
			"user_key",
			auditResourceID(keyView.ID),
			keyView.DisplayName,
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{"user_id": keyView.UserID},
		)
		writeJSON(w, http.StatusCreated, map[string]any{
			"user_key":    keyView,
			"raw_api_key": rawKey,
		})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminUserKeyByID(w http.ResponseWriter, r *http.Request, path string) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "revoke" {
		id, err := parseRouteID(parts[0])
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid user key id")
			return
		}
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := s.revokeManagedUserKey(id, adminActorFromContext(r.Context())); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to revoke user key")
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"user_key.revoke",
			"user_key",
			auditResourceID(id),
			auditResourceID(id),
			"success",
			RequestIDFromContext(r.Context()),
			nil,
		)
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
		return
	}

	if len(parts) == 1 && r.Method == http.MethodDelete {
		id, err := parseRouteID(parts[0])
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid user key id")
			return
		}
		if err := s.deleteManagedUserKey(id, adminActorFromContext(r.Context())); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to delete user key")
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"user_key.delete",
			"user_key",
			auditResourceID(id),
			auditResourceID(id),
			"success",
			RequestIDFromContext(r.Context()),
			nil,
		)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodPatch {
		id, err := parseRouteID(parts[0])
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid user key id")
			return
		}
		body, _ := io.ReadAll(r.Body)
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(body, &raw)
		costLimitRaw, hasCostLimit := raw["monthly_cost_limit_eur"]
		if !hasCostLimit {
			costLimitRaw, hasCostLimit = raw["monthly_cost_limit_usd"]
		}
		if hasCostLimit {
			var limitEUR *float64
			if costLimitRaw != nil {
				var v float64
				if err := json.Unmarshal(costLimitRaw, &v); err == nil && v > 0 {
					limitEUR = &v
				}
			}
			if err := s.setManagedUserKeyCostLimit(id, limitEUR); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to update cost limit")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			return
		}
		writeJSONError(w, http.StatusBadRequest, "no updatable fields provided")
		return
	}

	writeJSONError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		providers, err := s.listManagedProviderCredentials()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load providers")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
	case http.MethodPost:
		var payload struct {
			ProviderType string `json:"provider_type"`
			DisplayName  string `json:"display_name"`
			APIKey       string `json:"api_key"`
			BaseURL      string `json:"base_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		provider, models, err := s.createManagedProviderCredential(
			r.Context(),
			payload.ProviderType,
			payload.DisplayName,
			payload.APIKey,
			payload.BaseURL,
			adminActorFromContext(r.Context()),
		)
		if err != nil {
			s.writeProviderMutationError(w, r, "provider.create", err)
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"provider.create",
			"provider",
			auditResourceID(provider.ID),
			provider.DisplayName,
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{"provider_type": provider.ProviderType},
		)
		writeJSON(w, http.StatusCreated, map[string]any{
			"provider": provider,
			"models":   models,
		})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminProviderByID(w http.ResponseWriter, r *http.Request, path string) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	id, err := parseRouteID(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := s.deleteManagedProviderCredential(id, adminActorFromContext(r.Context())); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to delete provider")
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"provider.delete",
			"provider",
			auditResourceID(id),
			auditResourceID(id),
			"success",
			RequestIDFromContext(r.Context()),
			nil,
		)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(parts) == 2 && parts[1] == "rotate" {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var payload struct {
			APIKey string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		provider, models, err := s.rotateManagedProviderCredential(r.Context(), id, payload.APIKey, adminActorFromContext(r.Context()))
		if err != nil {
			s.writeProviderMutationError(w, r, "provider.rotate", err)
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"provider.rotate",
			"provider",
			auditResourceID(id),
			provider.DisplayName,
			"success",
			RequestIDFromContext(r.Context()),
			nil,
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": provider,
			"models":   models,
		})
		return
	}

	if len(parts) >= 2 && parts[1] == "models" {
		s.handleAdminProviderModels(w, r, id, parts[2:])
		return
	}

	writeJSONError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminProviderModels(w http.ResponseWriter, r *http.Request, id int64, extra []string) {
	if len(extra) == 0 {
		switch r.Method {
		case http.MethodGet:
			provider, models, err := s.listManagedProviderModels(id)
			if err != nil {
				writeJSONError(w, http.StatusNotFound, "provider not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"provider": provider,
				"models":   models,
			})
		case http.MethodPatch:
			var payload struct {
				EnabledModelIDs []string `json:"enabled_model_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid json request")
				return
			}
			provider, models, err := s.updateManagedProviderEnabledModels(id, payload.EnabledModelIDs)
			if err != nil {
				if errors.Is(err, errModelNotSheetAllowed) {
					writeJSONError(w, http.StatusBadRequest, err.Error())
				} else {
					writeJSONError(w, http.StatusInternalServerError, "failed to update provider models")
				}
				return
			}
			writeAuditEvent(
				s.management,
				adminActorFromContext(r.Context()),
				"provider.models.update",
				"provider",
				auditResourceID(id),
				provider.DisplayName,
				"success",
				RequestIDFromContext(r.Context()),
				map[string]string{"enabled_model_count": strconv.Itoa(len(payload.EnabledModelIDs))},
			)
			writeJSON(w, http.StatusOK, map[string]any{
				"provider": provider,
				"models":   models,
			})
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(extra) == 1 && extra[0] == "refresh" {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		provider, models, err := s.refreshManagedProviderModels(r.Context(), id, adminActorFromContext(r.Context()))
		if err != nil {
			s.writeProviderMutationError(w, r, "provider.models.refresh", err)
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"provider.models.refresh",
			"provider",
			auditResourceID(id),
			provider.DisplayName,
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{"model_count": strconv.Itoa(len(models))},
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": provider,
			"models":   models,
		})
		return
	}

	writeJSONError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminPIIPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		policies, err := s.listManagedPIIPolicies()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load pii policy")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"policies": policies})
	case http.MethodPatch:
		var payload struct {
			Policies []struct {
				EntityType string `json:"entity_type"`
				Enabled    bool   `json:"enabled"`
				Action     string `json:"action"`
			} `json:"policies"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		records := make([]piiPolicyRecord, 0, len(payload.Policies))
		for _, policy := range payload.Policies {
			records = append(records, piiPolicyRecord{
				EntityType: policy.EntityType,
				Enabled:    policy.Enabled,
				Action:     policy.Action,
			})
		}
		policies, err := s.updateManagedPIIPolicies(records, adminActorFromContext(r.Context()))
		if err != nil {
			writeAuditEvent(
				s.management,
				adminActorFromContext(r.Context()),
				"pii_policy.update",
				"pii_policy",
				"",
				"global",
				"failed",
				RequestIDFromContext(r.Context()),
				map[string]string{"reason": err.Error()},
			)
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"pii_policy.update",
			"pii_policy",
			"global",
			"global",
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{"policy_count": strconv.Itoa(len(records))},
		)
		writeJSON(w, http.StatusOK, map[string]any{"policies": policies})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminNSFWTerms(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		terms, err := s.listManagedNSFWBlockedTerms()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load nsfw terms")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"terms": terms})
	case http.MethodPost:
		var payload struct {
			Term    string `json:"term"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		term, err := s.createManagedNSFWBlockedTerm(payload.Term, enabled, adminActorFromContext(r.Context()))
		if err != nil {
			s.writeNSFWTermMutationError(w, r, "nsfw_term.create", "", err)
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"nsfw_term.create",
			"nsfw_term",
			auditResourceID(term.ID),
			auditResourceID(term.ID),
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{"enabled": strconv.FormatBool(term.Enabled)},
		)
		writeJSON(w, http.StatusCreated, map[string]any{"term": term})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminNSFWTermByID(w http.ResponseWriter, r *http.Request, path string) {
	id, err := parseRouteID(strings.Trim(path, "/"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid nsfw term id")
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var payload struct {
			Term    string `json:"term"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		record, found, err := s.management.getNSFWBlockedTermByID(id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load nsfw term")
			return
		}
		if !found {
			writeJSONError(w, http.StatusNotFound, "nsfw blocked term not found")
			return
		}
		enabled := record.Enabled
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		term, err := s.updateManagedNSFWBlockedTerm(id, payload.Term, enabled, adminActorFromContext(r.Context()))
		if err != nil {
			s.writeNSFWTermMutationError(w, r, "nsfw_term.update", auditResourceID(id), err)
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"nsfw_term.update",
			"nsfw_term",
			auditResourceID(term.ID),
			auditResourceID(term.ID),
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{"enabled": strconv.FormatBool(term.Enabled)},
		)
		writeJSON(w, http.StatusOK, map[string]any{"term": term})
	case http.MethodDelete:
		if err := s.deleteManagedNSFWBlockedTerm(id); err != nil {
			s.writeNSFWTermMutationError(w, r, "nsfw_term.delete", auditResourceID(id), err)
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"nsfw_term.delete",
			"nsfw_term",
			auditResourceID(id),
			auditResourceID(id),
			"success",
			RequestIDFromContext(r.Context()),
			nil,
		)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) writeProviderMutationError(w http.ResponseWriter, r *http.Request, action string, err error) {
	actor := adminActorFromContext(r.Context())
	requestID := RequestIDFromContext(r.Context())
	var validationErr providerValidationError
	switch {
	case errors.Is(err, errProviderSecretLocked):
		writeAuditEvent(s.management, actor, action, "provider", "", "", "failed", requestID, map[string]string{"reason": err.Error()})
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	case errors.As(err, &validationErr):
		writeAuditEvent(s.management, actor, action, "provider", "", "", "failed", requestID, map[string]string{"reason": err.Error()})
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		status := http.StatusInternalServerError
		message := "provider operation failed"
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "already in use") {
			status = http.StatusConflict
			message = err.Error()
		} else if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
			message = err.Error()
		} else if strings.Contains(err.Error(), "unsupported provider type") {
			status = http.StatusBadRequest
			message = err.Error()
		}
		writeAuditEvent(s.management, actor, action, "provider", "", "", "failed", requestID, map[string]string{"reason": err.Error()})
		writeJSONError(w, status, message)
	}
}

func (s *Server) writeNSFWTermMutationError(w http.ResponseWriter, r *http.Request, action, resourceID string, err error) {
	actor := adminActorFromContext(r.Context())
	requestID := RequestIDFromContext(r.Context())
	var validationErr nsfwBlockedTermValidationError
	switch {
	case errors.Is(err, errNSFWBlockedTermNotFound):
		writeAuditEvent(s.management, actor, action, "nsfw_term", resourceID, resourceID, "failed", requestID, map[string]string{"reason": err.Error()})
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.As(err, &validationErr):
		writeAuditEvent(s.management, actor, action, "nsfw_term", resourceID, resourceID, "failed", requestID, map[string]string{"reason": err.Error()})
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeAuditEvent(s.management, actor, action, "nsfw_term", resourceID, resourceID, "failed", requestID, map[string]string{"reason": err.Error()})
		writeJSONError(w, http.StatusInternalServerError, "nsfw term operation failed")
	}
}

func (s *Server) handleAdminRateLimiterConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.getManagedRateLimiterConfig()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load rate limiter config")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": cfg})
	case http.MethodPost:
		var payload struct {
			Enabled                   bool `json:"enabled"`
			RequestLimit              int  `json:"request_limit"`
			WindowSeconds             int  `json:"window_seconds"`
			QuarantineDurationSeconds int  `json:"quarantine_duration_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json request")
			return
		}
		if payload.RequestLimit <= 0 {
			writeJSONError(w, http.StatusBadRequest, "request_limit must be greater than 0")
			return
		}
		if payload.WindowSeconds <= 0 {
			writeJSONError(w, http.StatusBadRequest, "window_seconds must be greater than 0")
			return
		}
		if payload.QuarantineDurationSeconds <= 0 {
			writeJSONError(w, http.StatusBadRequest, "quarantine_duration_seconds must be greater than 0")
			return
		}
		cfg, err := s.saveManagedRateLimiterConfig(
			payload.Enabled,
			payload.RequestLimit,
			payload.WindowSeconds,
			payload.QuarantineDurationSeconds,
			adminActorFromContext(r.Context()),
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save rate limiter config")
			return
		}
		writeAuditEvent(
			s.management,
			adminActorFromContext(r.Context()),
			"rate_limiter_config.update",
			"rate_limiter_config",
			"global",
			"global",
			"success",
			RequestIDFromContext(r.Context()),
			map[string]string{
				"enabled":        strconv.FormatBool(cfg.Enabled),
				"request_limit":  strconv.Itoa(cfg.RequestLimit),
				"window_seconds": strconv.Itoa(cfg.WindowSeconds),
			},
		)
		writeJSON(w, http.StatusOK, map[string]any{"config": cfg})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminQuarantine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	quarantines, err := s.listManagedQuarantines()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load quarantines")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quarantines": quarantines})
}

func (s *Server) handleAdminQuarantineByUserID(w http.ResponseWriter, r *http.Request, userID string) {
	userID = strings.Trim(userID, "/")
	if userID == "" {
		writeJSONError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	actor := adminActorFromContext(r.Context())
	if err := s.unlockManagedQuarantine(userID, actor); err != nil {
		if errors.Is(err, errQuarantineNotFound) {
			writeJSONError(w, http.StatusNotFound, "no active quarantine found for user")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to unlock quarantine")
		return
	}
	writeAuditEvent(
		s.management,
		actor,
		"rate_limiter.quarantine_unlock",
		"user",
		userID,
		userID,
		"success",
		RequestIDFromContext(r.Context()),
		nil,
	)
	w.WriteHeader(http.StatusNoContent)
}

func parseRouteID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid route id")
	}
	return id, nil
}
