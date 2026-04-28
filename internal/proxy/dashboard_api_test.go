package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestDashboardLogsAndUsageEndpoints(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
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
	proxyReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating proxy request: %v", err)
	}
	proxyReq.Header.Set("Authorization", "Bearer tenant-key")
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyResp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected proxy status: %d", proxyResp.StatusCode)
	}

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logs status: %d", logsResp.StatusCode)
	}

	var logsPayload struct {
		UserID string `json:"user_id"`
		Logs   []struct {
			Model   string `json:"model"`
			Status  int    `json:"status"`
			Latency int64  `json:"latency_ms"`
			Total   *int64 `json:"total_tokens"`
		} `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsPayload); err != nil {
		t.Fatalf("failed to decode logs payload: %v", err)
	}
	if logsPayload.UserID != "tenant-a" {
		t.Fatalf("unexpected user_id: %q", logsPayload.UserID)
	}
	if len(logsPayload.Logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logsPayload.Logs))
	}
	if logsPayload.Logs[0].Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %q", logsPayload.Logs[0].Model)
	}
	if logsPayload.Logs[0].Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", logsPayload.Logs[0].Status)
	}
	if logsPayload.Logs[0].Latency < 0 {
		t.Fatalf("unexpected latency: %d", logsPayload.Logs[0].Latency)
	}
	if logsPayload.Logs[0].Total == nil || *logsPayload.Logs[0].Total != 5 {
		t.Fatalf("unexpected total tokens: %#v", logsPayload.Logs[0].Total)
	}

	usageResp, err := http.Get(gateway.URL + dashboardUsagePath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("usage request failed: %v", err)
	}
	defer usageResp.Body.Close()
	if usageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected usage status: %d", usageResp.StatusCode)
	}

	var usagePayload struct {
		UserID string `json:"user_id"`
		Usage  struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(usageResp.Body).Decode(&usagePayload); err != nil {
		t.Fatalf("failed to decode usage payload: %v", err)
	}
	if usagePayload.UserID != "tenant-a" {
		t.Fatalf("unexpected user_id: %q", usagePayload.UserID)
	}
	if usagePayload.Usage.PromptTokens != 2 || usagePayload.Usage.CompletionTokens != 3 || usagePayload.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage totals: %+v", usagePayload.Usage)
	}
}

func TestDashboardEndpointsRejectInvalidTenantKey(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?user_api_key=wrong-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 from logs endpoint, got %d", logsResp.StatusCode)
	}

	usageResp, err := http.Get(gateway.URL + dashboardUsagePath)
	if err != nil {
		t.Fatalf("usage request failed: %v", err)
	}
	defer usageResp.Body.Close()
	if usageResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 from usage endpoint, got %d", usageResp.StatusCode)
	}
}

func TestDashboardPlaygroundShowsProxyFailure(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"upstream failed"}`))
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

	reqPayload := []byte(`{"user_id":"tenant-a","model":"gpt-4o-mini","prompt":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+dashboardPlaygroundPath, bytes.NewReader(reqPayload))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playground request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var payload struct {
		ProxyStatus int `json:"proxy_status"`
		RawJSON     struct {
			Error string `json:"error"`
		} `json:"raw_json"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed decoding playground payload: %v", err)
	}
	if payload.ProxyStatus != http.StatusBadRequest {
		t.Fatalf("unexpected proxy_status: %d", payload.ProxyStatus)
	}
	if payload.RawJSON.Error != "upstream failed" {
		t.Fatalf("unexpected raw_json error: %q", payload.RawJSON.Error)
	}
}

func TestDashboardPlaygroundFailsClosedWithoutManagedProvider(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected upstream call to %s", r.URL.Path)
	}))
	defer upstream.Close()

	cfg := config.Config{
		UpstreamBaseURL: upstream.URL,
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqPayload := []byte(`{"user_id":"tenant-a","model":"gpt-4o-mini","prompt":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+dashboardPlaygroundPath, bytes.NewReader(reqPayload))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playground request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestDashboardPlaygroundRejectsRequestWithNoTenantHeader(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqPayload := []byte(`{"model":"gpt-4o-mini","prompt":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+dashboardPlaygroundPath, bytes.NewReader(reqPayload))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playground request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDashboardPlaygroundRejectsRequestWithInvalidTenantKey(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqPayload := []byte(`{"user_id":"unknown-user","model":"gpt-4o-mini","prompt":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+dashboardPlaygroundPath, bytes.NewReader(reqPayload))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playground request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDashboardPlaygroundRequestsAreIncludedInLogsAndUsage(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
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

	reqPayload := []byte(`{"user_id":"tenant-a","model":"gpt-4o-mini","prompt":"hello from playground"}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+dashboardPlaygroundPath, bytes.NewReader(reqPayload))
	if err != nil {
		t.Fatalf("failed creating playground request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playground request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logs status: %d", logsResp.StatusCode)
	}

	var logsPayload struct {
		Logs []struct {
			Model string `json:"model"`
			Total *int64 `json:"total_tokens"`
		} `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsPayload); err != nil {
		t.Fatalf("decode logs payload: %v", err)
	}
	if len(logsPayload.Logs) != 1 {
		t.Fatalf("expected 1 playground log, got %d", len(logsPayload.Logs))
	}
	if logsPayload.Logs[0].Model != "gpt-4o-mini" {
		t.Fatalf("unexpected logged model: %q", logsPayload.Logs[0].Model)
	}
	if logsPayload.Logs[0].Total == nil || *logsPayload.Logs[0].Total != 5 {
		t.Fatalf("unexpected logged total tokens: %#v", logsPayload.Logs[0].Total)
	}

	usageResp, err := http.Get(gateway.URL + dashboardUsagePath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("usage request failed: %v", err)
	}
	defer usageResp.Body.Close()
	if usageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected usage status: %d", usageResp.StatusCode)
	}

	var usagePayload struct {
		Usage struct {
			TotalTokens int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(usageResp.Body).Decode(&usagePayload); err != nil {
		t.Fatalf("decode usage payload: %v", err)
	}
	if usagePayload.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage total tokens: %d", usagePayload.Usage.TotalTokens)
	}

	overviewLogsResp, err := http.Get(gateway.URL + dashboardOverviewLogs)
	if err != nil {
		t.Fatalf("overview logs request failed: %v", err)
	}
	defer overviewLogsResp.Body.Close()
	if overviewLogsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview logs status: %d", overviewLogsResp.StatusCode)
	}

	var overviewLogsPayload struct {
		Users []struct {
			LogCount int `json:"log_count"`
		} `json:"users"`
	}
	if err := json.NewDecoder(overviewLogsResp.Body).Decode(&overviewLogsPayload); err != nil {
		t.Fatalf("decode overview logs payload: %v", err)
	}
	if len(overviewLogsPayload.Users) != 1 || overviewLogsPayload.Users[0].LogCount != 1 {
		t.Fatalf("unexpected overview logs payload: %+v", overviewLogsPayload.Users)
	}

	overviewUsageResp, err := http.Get(gateway.URL + dashboardOverviewUsage)
	if err != nil {
		t.Fatalf("overview usage request failed: %v", err)
	}
	defer overviewUsageResp.Body.Close()
	if overviewUsageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview usage status: %d", overviewUsageResp.StatusCode)
	}

	var overviewUsagePayload struct {
		Users []struct {
			Usage struct {
				TotalTokens int64 `json:"total_tokens"`
			} `json:"usage"`
		} `json:"users"`
	}
	if err := json.NewDecoder(overviewUsageResp.Body).Decode(&overviewUsagePayload); err != nil {
		t.Fatalf("decode overview usage payload: %v", err)
	}
	if len(overviewUsagePayload.Users) != 1 || overviewUsagePayload.Users[0].Usage.TotalTokens != 5 {
		t.Fatalf("unexpected overview usage payload: %+v", overviewUsagePayload.Users)
	}
}

func TestDashboardPlaygroundModelsReturnsOnlyEnabledSyncedModels(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	provider, found, err := server.management.getActiveProviderCredential()
	if err != nil {
		t.Fatalf("getActiveProviderCredential: %v", err)
	}
	if !found {
		t.Fatal("expected bootstrapped provider credential")
	}
	if _, err := server.management.updateProviderModelEnabled(provider.ID, []string{"gpt-4.1"}); err != nil {
		t.Fatalf("updateProviderModelEnabled: %v", err)
	}

	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	resp, err := http.Get(gateway.URL + dashboardPlaygroundModelsPath)
	if err != nil {
		t.Fatalf("playground model request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Models []dashboardPlaygroundModelView `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed decoding playground model payload: %v", err)
	}
	if len(payload.Models) != 1 {
		t.Fatalf("expected 1 enabled synced model, got %d", len(payload.Models))
	}
	if payload.Models[0].ProviderModelID != "gpt-4.1" {
		t.Fatalf("unexpected provider model id: %q", payload.Models[0].ProviderModelID)
	}
	if payload.Models[0].ProviderType != "openai" {
		t.Fatalf("unexpected provider type: %q", payload.Models[0].ProviderType)
	}
	if payload.Models[0].ProviderDisplayName == "" {
		t.Fatal("expected provider display name in playground model payload")
	}
}

func TestDashboardPlaygroundModelsReturnsEmptyWhenNoProviderIsConfigured(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	resp, err := http.Get(gateway.URL + dashboardPlaygroundModelsPath)
	if err != nil {
		t.Fatalf("playground model request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Models []dashboardPlaygroundModelView `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed decoding empty playground model payload: %v", err)
	}
	if len(payload.Models) != 0 {
		t.Fatalf("expected no playground models, got %d", len(payload.Models))
	}
}

func TestDashboardEndpointsSupportPeriodFilters(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
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
	proxyReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating proxy request: %v", err)
	}
	proxyReq.Header.Set("Authorization", "Bearer tenant-key")
	proxyReq.Header.Set("Content-Type", "application/json")
	if _, err := http.DefaultClient.Do(proxyReq); err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}

	from := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	to := time.Now().UTC().Add(1 * time.Minute).Format(time.RFC3339)
	query := url.Values{
		"user_api_key": []string{"tenant-key"},
		"from":         []string{from},
		"to":           []string{to},
	}.Encode()

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?" + query)
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logs status: %d", logsResp.StatusCode)
	}

	var logsPayload struct {
		Logs []json.RawMessage `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsPayload); err != nil {
		t.Fatalf("failed to decode logs payload: %v", err)
	}
	if len(logsPayload.Logs) == 0 {
		t.Fatal("expected logs in filtered period")
	}

	usageResp, err := http.Get(gateway.URL + dashboardUsagePath + "?" + query)
	if err != nil {
		t.Fatalf("usage request failed: %v", err)
	}
	defer usageResp.Body.Close()
	if usageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected usage status: %d", usageResp.StatusCode)
	}
}

func TestDashboardEndpointsRejectInvalidPeriodFilters(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	cases := []string{
		"?user_api_key=tenant-key&from=not-a-time&to=2026-01-01T00:00:00Z",
		"?user_api_key=tenant-key&from=2026-01-01T00:00:00Z&to=2026-01-01T00:00:00Z",
	}

	for _, query := range cases {
		logsResp, err := http.Get(gateway.URL + dashboardLogsPath + query)
		if err != nil {
			t.Fatalf("logs request failed: %v", err)
		}
		logsResp.Body.Close()
		if logsResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for logs query %q, got %d", query, logsResp.StatusCode)
		}

		usageResp, err := http.Get(gateway.URL + dashboardUsagePath + query)
		if err != nil {
			t.Fatalf("usage request failed: %v", err)
		}
		usageResp.Body.Close()
		if usageResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for usage query %q, got %d", query, usageResp.StatusCode)
		}
	}
}

func TestDashboardDataPersistsAcrossServerRestart(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)

	server1 := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway1 := httptest.NewServer(server1.Handler())

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`)
	proxyReq, err := http.NewRequest(http.MethodPost, gateway1.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating proxy request: %v", err)
	}
	proxyReq.Header.Set("Authorization", "Bearer tenant-key")
	proxyReq.Header.Set("Content-Type", "application/json")
	if _, err := http.DefaultClient.Do(proxyReq); err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}

	gateway1.Close()
	if err := server1.Close(); err != nil {
		t.Fatalf("failed closing first server: %v", err)
	}

	server2 := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway2 := httptest.NewServer(server2.Handler())
	defer gateway2.Close()

	logsResp, err := http.Get(gateway2.URL + dashboardLogsPath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	defer logsResp.Body.Close()

	var payload struct {
		Logs []json.RawMessage `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode logs payload: %v", err)
	}
	if len(payload.Logs) == 0 {
		t.Fatal("expected persisted logs after restart")
	}
}

func TestDashboardOverviewEndpointsReturnGroupedTenantData(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key-a": "tenant-a",
			"tenant-key-b": "tenant-b",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	for _, key := range []string{"tenant-key-a", "tenant-key-b"} {
		reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`)
		proxyReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("failed creating proxy request: %v", err)
		}
		proxyReq.Header.Set("Authorization", "Bearer "+key)
		proxyReq.Header.Set("Content-Type", "application/json")
		proxyResp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			t.Fatalf("proxy request failed: %v", err)
		}
		proxyResp.Body.Close()
		if proxyResp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected proxy status: %d", proxyResp.StatusCode)
		}
	}

	logsResp, err := http.Get(gateway.URL + dashboardOverviewLogs)
	if err != nil {
		t.Fatalf("overview logs request failed: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview logs status: %d", logsResp.StatusCode)
	}

	var logsPayload struct {
		Users []struct {
			UserID   string `json:"user_id"`
			KeyAlias string `json:"key_alias"`
			LogCount int    `json:"log_count"`
		} `json:"users"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsPayload); err != nil {
		t.Fatalf("failed to decode logs overview payload: %v", err)
	}
	if len(logsPayload.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(logsPayload.Users))
	}
	for _, user := range logsPayload.Users {
		if user.KeyAlias == "tenant-key-a" || user.KeyAlias == "tenant-key-b" {
			t.Fatalf("raw key leaked in key alias: %q", user.KeyAlias)
		}
		if user.LogCount < 1 {
			t.Fatalf("expected logs for user %q", user.UserID)
		}
	}

	usageResp, err := http.Get(gateway.URL + dashboardOverviewUsage)
	if err != nil {
		t.Fatalf("overview usage request failed: %v", err)
	}
	defer usageResp.Body.Close()
	if usageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview usage status: %d", usageResp.StatusCode)
	}

	var usagePayload struct {
		Users []struct {
			UserID   string `json:"user_id"`
			KeyAlias string `json:"key_alias"`
			Usage    struct {
				TotalTokens int64 `json:"total_tokens"`
			} `json:"usage"`
		} `json:"users"`
	}
	if err := json.NewDecoder(usageResp.Body).Decode(&usagePayload); err != nil {
		t.Fatalf("failed to decode usage overview payload: %v", err)
	}
	if len(usagePayload.Users) != 2 {
		t.Fatalf("expected 2 users in usage overview, got %d", len(usagePayload.Users))
	}
	for _, user := range usagePayload.Users {
		if user.KeyAlias == "tenant-key-a" || user.KeyAlias == "tenant-key-b" {
			t.Fatalf("raw key leaked in usage key alias: %q", user.KeyAlias)
		}
		if user.Usage.TotalTokens < 1 {
			t.Fatalf("expected non-zero usage for user %q", user.UserID)
		}
	}
}

func TestDashboardEndpointsExposePIISummariesWithoutRawValues(t *testing.T) {
	t.Parallel()

	const (
		requestEmail  = "alice@example.com"
		responsePhone = "+1 (202) 555-0199"
	)

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","choices":[{"message":{"role":"assistant","content":"Call ` + responsePhone + `"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
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

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Email me at ` + requestEmail + `"}]}`)
	proxyReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating proxy request: %v", err)
	}
	proxyReq.Header.Set("Authorization", "Bearer tenant-key")
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyResp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected proxy status: %d", proxyResp.StatusCode)
	}

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	logsBody, err := io.ReadAll(logsResp.Body)
	logsResp.Body.Close()
	if err != nil {
		t.Fatalf("read logs body: %v", err)
	}
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logs status: %d", logsResp.StatusCode)
	}
	if strings.Contains(string(logsBody), requestEmail) || strings.Contains(string(logsBody), responsePhone) {
		t.Fatalf("dashboard logs payload leaked raw pii: %s", string(logsBody))
	}

	var logsPayload struct {
		Logs []struct {
			PIISummary      *piiRequestSummary        `json:"pii_summary,omitempty"`
			SecuritySummary *dashboardSecuritySummary `json:"security_summary,omitempty"`
		} `json:"logs"`
	}
	if err := json.Unmarshal(logsBody, &logsPayload); err != nil {
		t.Fatalf("decode logs payload: %v", err)
	}
	if len(logsPayload.Logs) != 1 || logsPayload.Logs[0].PIISummary == nil {
		t.Fatalf("expected pii summary on logs payload, got %+v", logsPayload.Logs)
	}
	if logsPayload.Logs[0].SecuritySummary == nil || len(logsPayload.Logs[0].SecuritySummary.Labels) != 1 || logsPayload.Logs[0].SecuritySummary.Labels[0] != "PII" {
		t.Fatalf("expected security summary with PII label, got %+v", logsPayload.Logs[0].SecuritySummary)
	}
	if logsPayload.Logs[0].PIISummary.IngressFindingCount != 1 {
		t.Fatalf("unexpected ingress finding count: %d", logsPayload.Logs[0].PIISummary.IngressFindingCount)
	}
	if logsPayload.Logs[0].PIISummary.EgressFindingCount != 1 {
		t.Fatalf("unexpected egress finding count: %d", logsPayload.Logs[0].PIISummary.EgressFindingCount)
	}

	usageResp, err := http.Get(gateway.URL + dashboardUsagePath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("usage request failed: %v", err)
	}
	usageBody, err := io.ReadAll(usageResp.Body)
	usageResp.Body.Close()
	if err != nil {
		t.Fatalf("read usage body: %v", err)
	}
	if usageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected usage status: %d", usageResp.StatusCode)
	}
	if strings.Contains(string(usageBody), requestEmail) || strings.Contains(string(usageBody), responsePhone) {
		t.Fatalf("dashboard usage payload leaked raw pii: %s", string(usageBody))
	}

	var usagePayload struct {
		PIISummary      piiTenantSummary         `json:"pii_summary"`
		SecuritySummary dashboardSecuritySummary `json:"security_summary"`
	}
	if err := json.Unmarshal(usageBody, &usagePayload); err != nil {
		t.Fatalf("decode usage payload: %v", err)
	}
	if usagePayload.PIISummary.FlaggedRequestCount != 1 {
		t.Fatalf("unexpected flagged request count: %d", usagePayload.PIISummary.FlaggedRequestCount)
	}
	if len(usagePayload.SecuritySummary.Labels) != 1 || usagePayload.SecuritySummary.Labels[0] != "PII" {
		t.Fatalf("expected usage security summary with PII label, got %+v", usagePayload.SecuritySummary)
	}

	overviewLogsResp, err := http.Get(gateway.URL + dashboardOverviewLogs)
	if err != nil {
		t.Fatalf("overview logs request failed: %v", err)
	}
	overviewLogsBody, err := io.ReadAll(overviewLogsResp.Body)
	overviewLogsResp.Body.Close()
	if err != nil {
		t.Fatalf("read overview logs body: %v", err)
	}
	if overviewLogsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview logs status: %d", overviewLogsResp.StatusCode)
	}
	if strings.Contains(string(overviewLogsBody), requestEmail) || strings.Contains(string(overviewLogsBody), responsePhone) {
		t.Fatalf("overview logs payload leaked raw pii: %s", string(overviewLogsBody))
	}

	var overviewLogsPayload struct {
		Users []struct {
			Logs []struct {
				PIISummary *piiRequestSummary `json:"pii_summary,omitempty"`
			} `json:"logs"`
		} `json:"users"`
	}
	if err := json.Unmarshal(overviewLogsBody, &overviewLogsPayload); err != nil {
		t.Fatalf("decode overview logs payload: %v", err)
	}
	if len(overviewLogsPayload.Users) != 1 || len(overviewLogsPayload.Users[0].Logs) != 1 {
		t.Fatalf("unexpected overview logs payload: %+v", overviewLogsPayload)
	}
	if overviewLogsPayload.Users[0].Logs[0].PIISummary == nil {
		t.Fatal("expected per-request pii summary in overview logs payload")
	}

	overviewUsageResp, err := http.Get(gateway.URL + dashboardOverviewUsage)
	if err != nil {
		t.Fatalf("overview usage request failed: %v", err)
	}
	overviewUsageBody, err := io.ReadAll(overviewUsageResp.Body)
	overviewUsageResp.Body.Close()
	if err != nil {
		t.Fatalf("read overview usage body: %v", err)
	}
	if overviewUsageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview usage status: %d", overviewUsageResp.StatusCode)
	}
	if strings.Contains(string(overviewUsageBody), requestEmail) || strings.Contains(string(overviewUsageBody), responsePhone) {
		t.Fatalf("overview usage payload leaked raw pii: %s", string(overviewUsageBody))
	}

	var overviewUsagePayload struct {
		Users []struct {
			PIISummary      piiTenantSummary         `json:"pii_summary"`
			SecuritySummary dashboardSecuritySummary `json:"security_summary"`
		} `json:"users"`
	}
	if err := json.Unmarshal(overviewUsageBody, &overviewUsagePayload); err != nil {
		t.Fatalf("decode overview usage payload: %v", err)
	}
	if len(overviewUsagePayload.Users) != 1 {
		t.Fatalf("expected 1 user in overview usage payload, got %d", len(overviewUsagePayload.Users))
	}
	if overviewUsagePayload.Users[0].PIISummary.FlaggedRequestCount != 1 {
		t.Fatalf("unexpected overview flagged request count: %d", overviewUsagePayload.Users[0].PIISummary.FlaggedRequestCount)
	}
	if len(overviewUsagePayload.Users[0].SecuritySummary.Labels) != 1 || overviewUsagePayload.Users[0].SecuritySummary.Labels[0] != "PII" {
		t.Fatalf("expected overview usage security summary with PII label, got %+v", overviewUsagePayload.Users[0].SecuritySummary)
	}
}

func TestDashboardLogsOmitRawNSFWPromptText(t *testing.T) {
	t.Parallel()

	const blockedPhrase = "adult content"

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected upstream call for blocked nsfw request")
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if _, err := server.createManagedNSFWBlockedTerm(blockedPhrase, true, "tester"); err != nil {
		t.Fatalf("createManagedNSFWBlockedTerm: %v", err)
	}

	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"please send ` + blockedPhrase + ` now"}]}`)
	proxyReq, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating proxy request: %v", err)
	}
	proxyReq.Header.Set("Authorization", "Bearer tenant-key")
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyResp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected proxy status: %d", proxyResp.StatusCode)
	}

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	logsBody, err := io.ReadAll(logsResp.Body)
	logsResp.Body.Close()
	if err != nil {
		t.Fatalf("read logs body: %v", err)
	}
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logs status: %d", logsResp.StatusCode)
	}
	if strings.Contains(string(logsBody), blockedPhrase) {
		t.Fatalf("dashboard logs payload leaked raw nsfw prompt text: %s", string(logsBody))
	}
	if !strings.Contains(string(logsBody), `"labels":["NSFW"]`) {
		t.Fatalf("expected NSFW security label in logs payload, got %s", string(logsBody))
	}

	overviewUsageResp, err := http.Get(gateway.URL + dashboardOverviewUsage)
	if err != nil {
		t.Fatalf("overview usage request failed: %v", err)
	}
	overviewUsageBody, err := io.ReadAll(overviewUsageResp.Body)
	overviewUsageResp.Body.Close()
	if err != nil {
		t.Fatalf("read overview usage body: %v", err)
	}
	if overviewUsageResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview usage status: %d", overviewUsageResp.StatusCode)
	}
	if strings.Contains(string(overviewUsageBody), blockedPhrase) {
		t.Fatalf("overview usage payload leaked raw nsfw prompt text: %s", string(overviewUsageBody))
	}

	var overviewUsagePayload struct {
		Users []struct {
			SecuritySummary dashboardSecuritySummary `json:"security_summary"`
		} `json:"users"`
	}
	if err := json.Unmarshal(overviewUsageBody, &overviewUsagePayload); err != nil {
		t.Fatalf("decode overview usage payload: %v", err)
	}
	if len(overviewUsagePayload.Users) != 1 {
		t.Fatalf("expected 1 user in overview usage payload, got %d", len(overviewUsagePayload.Users))
	}
	if len(overviewUsagePayload.Users[0].SecuritySummary.Labels) != 1 || overviewUsagePayload.Users[0].SecuritySummary.Labels[0] != "NSFW" {
		t.Fatalf("expected NSFW security summary in overview usage payload, got %+v", overviewUsagePayload.Users[0].SecuritySummary)
	}

	outcomeRows, err := server.storage.query(`
SELECT matched_policy_id
FROM guardrail_outcomes
WHERE tenant_id = ?
`, "tenant-a")
	if err != nil {
		t.Fatalf("query guardrail outcomes: %v", err)
	}
	defer outcomeRows.Close()

	for outcomeRows.Next() {
		var matchedPolicyID string
		if err := outcomeRows.Scan(&matchedPolicyID); err != nil {
			t.Fatalf("scan guardrail outcome: %v", err)
		}
		if matchedPolicyID == blockedPhrase {
			t.Fatalf("persisted outcome leaked raw blocked phrase: %q", matchedPolicyID)
		}
	}
}

func TestDashboardOverviewEndpointsRejectInvalidPeriodFilters(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	query := "?from=2026-01-01T00:00:00Z&to=2026-01-01T00:00:00Z"
	for _, path := range []string{dashboardOverviewLogs, dashboardOverviewUsage} {
		resp, err := http.Get(gateway.URL + path + query)
		if err != nil {
			t.Fatalf("overview request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.StatusCode)
		}
	}
}
