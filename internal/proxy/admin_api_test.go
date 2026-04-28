package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestAdminAPIAllowsManagementWithoutSeparateAdminAuthorization(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	resp, err := http.Get(gateway.URL + adminAPIManagementPath + "/tenant-keys")
	if err != nil {
		t.Fatalf("admin request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAdminPIIPolicyLifecycle(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	getResp := performAdminJSONRequest(t, gateway.URL, http.MethodGet, "/pii-policy", nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from pii policy list, got %d", getResp.StatusCode)
	}
	var getPayload struct {
		Policies []adminPIIPolicyView `json:"policies"`
	}
	decodeResponse(t, getResp.Body, &getPayload)
	if len(getPayload.Policies) != len(supportedPIIEntityTypes()) {
		t.Fatalf("expected %d default pii policies, got %d", len(supportedPIIEntityTypes()), len(getPayload.Policies))
	}

	patchPolicies := make([]map[string]any, 0, len(getPayload.Policies))
	for _, policy := range getPayload.Policies {
		action := policy.Action
		if policy.EntityType == "email_address" {
			action = piiActionBlock
		}
		patchPolicies = append(patchPolicies, map[string]any{
			"entity_type": policy.EntityType,
			"enabled":     policy.Enabled,
			"action":      action,
		})
	}
	patchResp := performAdminJSONRequest(t, gateway.URL, http.MethodPatch, "/pii-policy", map[string]any{
		"policies": patchPolicies,
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from pii policy patch, got %d", patchResp.StatusCode)
	}
	var patchPayload struct {
		Policies []adminPIIPolicyView `json:"policies"`
	}
	decodeResponse(t, patchResp.Body, &patchPayload)

	var foundEmail bool
	for _, policy := range patchPayload.Policies {
		if policy.EntityType != "email_address" {
			continue
		}
		foundEmail = true
		if policy.Action != piiActionBlock {
			t.Fatalf("expected email_address policy to be blocked, got %q", policy.Action)
		}
		if policy.DisplayName == "" {
			t.Fatal("expected policy display_name to be populated")
		}
	}
	if !foundEmail {
		t.Fatal("expected patched email_address policy in response")
	}

	rows, err := server.storage.query(`SELECT action, status, details_json FROM audit_events WHERE action = ?`, "pii_policy.update")
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	defer rows.Close()

	var auditCount int
	for rows.Next() {
		auditCount++
		var (
			action  string
			status  string
			details string
		)
		if err := rows.Scan(&action, &status, &details); err != nil {
			t.Fatalf("scan audit event: %v", err)
		}
		if action != "pii_policy.update" {
			t.Fatalf("unexpected audit action: %q", action)
		}
		if status != "success" {
			t.Fatalf("expected success audit status, got %q", status)
		}
		if !strings.Contains(details, "policy_count") {
			t.Fatalf("expected policy_count in audit details, got %q", details)
		}
	}
	if auditCount == 0 {
		t.Fatal("expected at least one audit event for pii policy update")
	}
}

func TestAdminNSFWTermLifecycle(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	listResp := performAdminJSONRequest(t, gateway.URL, http.MethodGet, "/nsfw-terms", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from nsfw term list, got %d", listResp.StatusCode)
	}
	var listPayload struct {
		Terms []adminNSFWBlockedTermView `json:"terms"`
	}
	decodeResponse(t, listResp.Body, &listPayload)
	if len(listPayload.Terms) != 0 {
		t.Fatalf("expected no default nsfw terms, got %d", len(listPayload.Terms))
	}

	createResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/nsfw-terms", map[string]any{
		"term":    "Adult Content",
		"enabled": true,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from nsfw term create, got %d", createResp.StatusCode)
	}
	var createPayload struct {
		Term adminNSFWBlockedTermView `json:"term"`
	}
	decodeResponse(t, createResp.Body, &createPayload)
	if createPayload.Term.ID <= 0 {
		t.Fatalf("expected persisted term id, got %d", createPayload.Term.ID)
	}
	if createPayload.Term.Term != "Adult Content" {
		t.Fatalf("unexpected persisted term: %q", createPayload.Term.Term)
	}

	duplicateResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/nsfw-terms", map[string]any{
		"term":    " adult   content ",
		"enabled": true,
	})
	if duplicateResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 from duplicate nsfw term create, got %d", duplicateResp.StatusCode)
	}

	updateResp := performAdminJSONRequest(t, gateway.URL, http.MethodPatch, "/nsfw-terms/"+intToString(createPayload.Term.ID), map[string]any{
		"term":    "Adult Phrase",
		"enabled": false,
	})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from nsfw term update, got %d", updateResp.StatusCode)
	}
	var updatePayload struct {
		Term adminNSFWBlockedTermView `json:"term"`
	}
	decodeResponse(t, updateResp.Body, &updatePayload)
	if updatePayload.Term.Enabled {
		t.Fatal("expected updated nsfw term to be disabled")
	}

	deleteResp := performAdminJSONRequest(t, gateway.URL, http.MethodDelete, "/nsfw-terms/"+intToString(createPayload.Term.ID), nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from nsfw term delete, got %d", deleteResp.StatusCode)
	}
	deleteResp.Body.Close()

	rows, err := server.storage.query(`SELECT action, status, details_json FROM audit_events WHERE action LIKE 'nsfw_term.%' ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query nsfw audit events: %v", err)
	}
	defer rows.Close()

	var statuses []string
	for rows.Next() {
		var (
			action  string
			status  string
			details string
		)
		if err := rows.Scan(&action, &status, &details); err != nil {
			t.Fatalf("scan nsfw audit event: %v", err)
		}
		statuses = append(statuses, action+":"+status)
		if strings.Contains(details, "Adult Content") || strings.Contains(details, "Adult Phrase") {
			t.Fatalf("expected audit details to avoid raw term text, got %q", details)
		}
	}
	if len(statuses) < 4 {
		t.Fatalf("expected create, failed create, update, and delete audit events, got %v", statuses)
	}
}

func TestAdminTenantKeyLifecycleAndManagedProxyAuth(t *testing.T) {
	t.Parallel()

	upstream := managedProviderTestServer(t)
	defer upstream.Close()

	cfg := config.Config{
		SecretMasterKey:          "master-secret",
		OpenAIBaseURL:            upstream.URL,
		UpstreamTimeout:          2 * time.Second,
		LegacyFallback:           false,
		LegacyFallbackConfigured: true,
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	providerResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/providers", map[string]any{
		"provider_type": "openai",
		"display_name":  "OpenAI Primary",
		"api_key":       "provider-secret",
	})
	if providerResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from provider create, got %d", providerResp.StatusCode)
	}
	var providerPayload struct {
		Provider adminProviderCredentialView `json:"provider"`
		Models   []adminProviderModelView    `json:"models"`
	}
	decodeResponse(t, providerResp.Body, &providerPayload)
	if providerPayload.Provider.MaskedKey == "provider-secret" {
		t.Fatal("provider list response leaked full provider secret")
	}
	if len(providerPayload.Models) == 0 {
		t.Fatal("expected synced models in provider create response")
	}

	tenantResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/tenant-keys", map[string]any{
		"display_name": "Tenant A",
	})
	if tenantResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from tenant key create, got %d", tenantResp.StatusCode)
	}
	var tenantPayload struct {
		UserKey   adminUserKeyView `json:"user_key"`
		RawAPIKey string           `json:"raw_api_key"`
	}
	decodeResponse(t, tenantResp.Body, &tenantPayload)
	if tenantPayload.RawAPIKey == "" {
		t.Fatal("expected raw tenant api key in create response")
	}
	if tenantPayload.UserKey.UserID == "" {
		t.Fatal("expected generated user_id in create response")
	}
	if !strings.HasPrefix(tenantPayload.UserKey.UserID, "user-") {
		t.Fatalf("expected generated user_id prefix, got %q", tenantPayload.UserKey.UserID)
	}
	if tenantPayload.UserKey.MaskedKey == tenantPayload.RawAPIKey {
		t.Fatal("expected masked tenant key in metadata response")
	}

	listResp := performAdminJSONRequest(t, gateway.URL, http.MethodGet, "/tenant-keys", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from tenant key list, got %d", listResp.StatusCode)
	}
	body, err := io.ReadAll(listResp.Body)
	if err != nil {
		t.Fatalf("failed reading tenant key list body: %v", err)
	}
	if strings.Contains(string(body), tenantPayload.RawAPIKey) {
		t.Fatal("tenant key list leaked full raw tenant key")
	}

	proxyReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("failed creating proxy request: %v", err)
	}
	proxyReq.Header.Set("Authorization", "Bearer "+tenantPayload.RawAPIKey)
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyResp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from managed proxy request, got %d", proxyResp.StatusCode)
	}

	revokeResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/tenant-keys/"+intToString(tenantPayload.UserKey.ID)+"/revoke", nil)
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from revoke, got %d", revokeResp.StatusCode)
	}

	proxyReq2, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("failed creating second proxy request: %v", err)
	}
	proxyReq2.Header.Set("Authorization", "Bearer "+tenantPayload.RawAPIKey)
	proxyReq2.Header.Set("Content-Type", "application/json")
	proxyResp2, err := http.DefaultClient.Do(proxyReq2)
	if err != nil {
		t.Fatalf("second proxy request failed: %v", err)
	}
	proxyResp2.Body.Close()
	if proxyResp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revoke, got %d", proxyResp2.StatusCode)
	}

	auditRows, err := server.storage.query(`SELECT details_json FROM audit_events ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("failed querying audit events: %v", err)
	}
	defer auditRows.Close()
	for auditRows.Next() {
		var details string
		if err := auditRows.Scan(&details); err != nil {
			t.Fatalf("failed scanning audit details: %v", err)
		}
		if strings.Contains(details, "provider-secret") || strings.Contains(details, tenantPayload.RawAPIKey) {
			t.Fatalf("audit event leaked secret data: %s", details)
		}
	}
}

func TestProviderModelManagementRejectsDisabledModelAndPreservesCatalogOnRefreshFailure(t *testing.T) {
	t.Parallel()

	var modelRefreshFailures atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if modelRefreshFailures.Load() {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"error":"refresh failed"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini","owned_by":"openai"},{"id":"gpt-4.1","owned_by":"openai"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := config.Config{
		SecretMasterKey:          "master-secret",
		OpenAIBaseURL:            upstream.URL,
		UpstreamTimeout:          2 * time.Second,
		LegacyFallback:           false,
		LegacyFallbackConfigured: true,
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	providerResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/providers", map[string]any{
		"provider_type": "openai",
		"display_name":  "OpenAI Primary",
		"api_key":       "provider-secret",
	})
	if providerResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from provider create, got %d", providerResp.StatusCode)
	}
	var providerPayload struct {
		Provider adminProviderCredentialView `json:"provider"`
	}
	decodeResponse(t, providerResp.Body, &providerPayload)

	tenantResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/tenant-keys", map[string]any{
		"display_name": "Tenant A",
	})
	if tenantResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from tenant key create, got %d", tenantResp.StatusCode)
	}
	var tenantPayload struct {
		RawAPIKey string `json:"raw_api_key"`
	}
	decodeResponse(t, tenantResp.Body, &tenantPayload)

	patchResp := performAdminJSONRequest(t, gateway.URL, http.MethodPatch, "/providers/"+intToString(providerPayload.Provider.ID)+"/models", map[string]any{
		"enabled_model_ids": []string{"gpt-4.1"},
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from model patch, got %d", patchResp.StatusCode)
	}

	rejectedReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("failed creating rejected proxy request: %v", err)
	}
	rejectedReq.Header.Set("Authorization", "Bearer "+tenantPayload.RawAPIKey)
	rejectedReq.Header.Set("Content-Type", "application/json")
	rejectedResp, err := http.DefaultClient.Do(rejectedReq)
	if err != nil {
		t.Fatalf("rejected proxy request failed: %v", err)
	}
	rejectedResp.Body.Close()
	if rejectedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for disabled model, got %d", rejectedResp.StatusCode)
	}

	acceptedReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("failed creating accepted proxy request: %v", err)
	}
	acceptedReq.Header.Set("Authorization", "Bearer "+tenantPayload.RawAPIKey)
	acceptedReq.Header.Set("Content-Type", "application/json")
	acceptedResp, err := http.DefaultClient.Do(acceptedReq)
	if err != nil {
		t.Fatalf("accepted proxy request failed: %v", err)
	}
	acceptedResp.Body.Close()
	if acceptedResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for enabled model, got %d", acceptedResp.StatusCode)
	}

	modelRefreshFailures.Store(true)
	refreshResp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/providers/"+intToString(providerPayload.Provider.ID)+"/models/refresh", nil)
	if refreshResp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 from failed refresh, got %d", refreshResp.StatusCode)
	}

	modelsResp := performAdminJSONRequest(t, gateway.URL, http.MethodGet, "/providers/"+intToString(providerPayload.Provider.ID)+"/models", nil)
	if modelsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from model list, got %d", modelsResp.StatusCode)
	}
	var modelsPayload struct {
		Models []adminProviderModelView `json:"models"`
	}
	decodeResponse(t, modelsResp.Body, &modelsPayload)
	if len(modelsPayload.Models) != 2 {
		t.Fatalf("expected previous catalog to remain after failed refresh, got %d models", len(modelsPayload.Models))
	}
}

func TestAdminTenantKeyCreateRejectsCallerSuppliedUserID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	resp := performAdminJSONRequest(t, gateway.URL, http.MethodPost, "/tenant-keys", map[string]any{
		"user_id":      "user-manual",
		"display_name": "Manual User",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 from tenant key create with caller-supplied user_id, got %d", resp.StatusCode)
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error != "user_id is assigned by the server" {
		t.Fatalf("unexpected error message: %q", payload.Error)
	}
}

func managedProviderTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini","owned_by":"openai"},{"id":"gpt-4.1","owned_by":"openai"}]}`))
		case "/v1/chat/completions":
			if got := r.Header.Get("Authorization"); got != "Bearer provider-secret" {
				t.Fatalf("unexpected provider auth header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func performAdminJSONRequest(t *testing.T, baseURL, method, path string, payload any) *http.Response {
	t.Helper()

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal request payload: %v", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, baseURL+adminAPIManagementPath+path, body)
	if err != nil {
		t.Fatalf("failed to create admin request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin request failed: %v", err)
	}
	return resp
}

func decodeResponse(t *testing.T, body io.ReadCloser, target any) {
	t.Helper()
	defer body.Close()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func intToString(value int64) string {
	return strconv.FormatInt(value, 10)
}
