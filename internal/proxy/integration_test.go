package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestChatCompletionsProxySuccessPassThrough(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-key" {
			t.Fatalf("unexpected upstream auth header: %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed reading upstream request: %v", err)
		}
		expectedBody := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`
		if string(body) != expectedBody {
			t.Fatalf("unexpected upstream body: %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	expectedRespBody := `{"id":"resp_1","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`
	if string(respBody) != expectedRespBody {
		t.Fatalf("unexpected response body: %s", string(respBody))
	}
}

func TestChatCompletionsProxyPassesThroughUpstreamError(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"upstream validation failure"}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	expectedRespBody := `{"error":"upstream validation failure"}`
	if string(respBody) != expectedRespBody {
		t.Fatalf("unexpected response body: %s", string(respBody))
	}
}

func TestChatCompletionsProxyReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"late"}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 50 * time.Millisecond,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	if !strings.Contains(string(respBody), "upstream timeout") {
		t.Fatalf("unexpected response body: %s", string(respBody))
	}
}

func TestChatCompletionsProxyFailsClosedWithoutManagedProvider(t *testing.T) {
	t.Parallel()

	var upstreamCalled atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			upstreamCalled.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	}))
	defer upstream.Close()

	cfg := config.Config{
		UpstreamBaseURL: upstream.URL,
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	if upstreamCalled.Load() {
		t.Fatal("expected proxy to fail closed before upstream call")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	if !strings.Contains(string(body), "provider configuration unavailable") {
		t.Fatalf("unexpected response body: %s", string(body))
	}
}

func TestChatCompletionsProxyObservesIngressAndEgressPII(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","choices":[{"message":{"role":"assistant","content":"Call me at +1 (202) 555-0199."}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Contact me at Alice@example.com"}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", upstreamCalls.Load())
	}

	summary, err := server.dashboard.piiSummary("tenant-a", dashboardQuery{})
	if err != nil {
		t.Fatalf("piiSummary: %v", err)
	}
	if summary.FlaggedRequestCount != 1 {
		t.Fatalf("expected 1 flagged request, got %d", summary.FlaggedRequestCount)
	}
	if summary.IngressFindingCount != 1 {
		t.Fatalf("expected 1 ingress finding, got %d", summary.IngressFindingCount)
	}
	if summary.EgressFindingCount != 1 {
		t.Fatalf("expected 1 egress finding, got %d", summary.EgressFindingCount)
	}
	if !strings.Contains(strings.Join(summary.EntityTypes, ","), "email_address") {
		t.Fatalf("expected email_address entity type in summary, got %v", summary.EntityTypes)
	}
	if !strings.Contains(strings.Join(summary.EntityTypes, ","), "phone_number") {
		t.Fatalf("expected phone_number entity type in summary, got %v", summary.EntityTypes)
	}
}

func TestChatCompletionsProxyBlocksIngressPIIWithoutUpstreamCall(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if _, err := server.management.savePIIPolicies([]piiPolicyRecord{
		{
			EntityType: "credit_card",
			Enabled:    true,
			Action:     piiActionObserve,
			UpdatedBy:  "test",
			UpdatedAt:  time.Now().UTC(),
		},
		{
			EntityType: "email_address",
			Enabled:    true,
			Action:     piiActionBlock,
			UpdatedBy:  "test",
			UpdatedAt:  time.Now().UTC(),
		},
		{
			EntityType: "phone_number",
			Enabled:    true,
			Action:     piiActionObserve,
			UpdatedBy:  "test",
			UpdatedAt:  time.Now().UTC(),
		},
	}); err != nil {
		t.Fatalf("savePIIPolicies: %v", err)
	}

	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Reach me at blockme@example.com"}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("expected no upstream calls, got %d", upstreamCalls.Load())
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if !strings.Contains(string(body), "request blocked by pii policy") {
		t.Fatalf("unexpected response body: %s", string(body))
	}

	logs, err := server.dashboard.logs("tenant-a", dashboardQuery{Limit: 10})
	if err != nil {
		t.Fatalf("dashboard.logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 persisted log for blocked request, got %d", len(logs))
	}
	if logs[0].Status != http.StatusBadRequest {
		t.Fatalf("expected blocked log status 400, got %d", logs[0].Status)
	}
	if logs[0].PIISummary == nil {
		t.Fatal("expected pii summary on blocked request log")
	}
	if got := logs[0].PIISummary.Actions; len(got) != 1 || got[0] != piiActionBlock {
		t.Fatalf("expected block action on persisted summary, got %v", got)
	}
}

func TestChatCompletionsProxyBlocksNSFWTermsWithoutUpstreamCall(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if _, err := server.createManagedNSFWBlockedTerm("adult content", true, "tester"); err != nil {
		t.Fatalf("createManagedNSFWBlockedTerm: %v", err)
	}

	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"This ADULT   content should be blocked."}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("expected no upstream calls, got %d", upstreamCalls.Load())
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if !strings.Contains(string(body), "request blocked by content policy") {
		t.Fatalf("unexpected response body: %s", string(body))
	}
	if strings.Contains(string(body), "adult content") {
		t.Fatalf("response leaked raw blocked term: %s", string(body))
	}

	rows, err := server.storage.query(`
SELECT guardrail_type, action, matched_policy_id
FROM guardrail_outcomes
WHERE tenant_id = ?
`, "tenant-a")
	if err != nil {
		t.Fatalf("query guardrail outcomes: %v", err)
	}
	defer rows.Close()

	var outcomeCount int
	for rows.Next() {
		outcomeCount++
		var (
			guardrailType   string
			action          string
			matchedPolicyID string
		)
		if err := rows.Scan(&guardrailType, &action, &matchedPolicyID); err != nil {
			t.Fatalf("scan guardrail outcome: %v", err)
		}
		if guardrailType != guardrailTypeNSFWKeyword {
			t.Fatalf("unexpected guardrail type: %q", guardrailType)
		}
		if action != nsfwActionBlock {
			t.Fatalf("unexpected guardrail action: %q", action)
		}
		if matchedPolicyID == "" {
			t.Fatal("expected matched policy id")
		}
		if matchedPolicyID == "adult content" {
			t.Fatalf("expected matched policy id, got raw term %q", matchedPolicyID)
		}
	}
	if outcomeCount != 1 {
		t.Fatalf("expected 1 persisted guardrail outcome, got %d", outcomeCount)
	}
}
