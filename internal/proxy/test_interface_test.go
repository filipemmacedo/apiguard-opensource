package proxy

import (
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

func TestTestInterfaceRouteDisabledByDefault(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
		EnableTestUI: false,
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	resp, err := http.Get(gateway.URL + testInterfacePath)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTestInterfaceRouteEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
		EnableTestUI: true,
	}
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	resp, err := http.Get(gateway.URL + testInterfacePath)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "User ID") {
		t.Fatalf("expected user id input in page, got %s", bodyText)
	}
	if !strings.Contains(bodyText, "textarea") {
		t.Fatalf("expected prompt textarea in page, got %s", bodyText)
	}
}

func TestTestInterfaceSubmitSuccess(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
		EnableTestUI: true,
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	values := url.Values{}
	values.Set("user_id", "tenant-a")
	values.Set("prompt", "hello")
	resp, err := http.PostForm(gateway.URL+testInterfaceSubmitPath, values)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "Proxy Status: 200") {
		t.Fatalf("expected proxy status in response, got %s", bodyText)
	}
	if !strings.Contains(bodyText, "resp_1") {
		t.Fatalf("expected raw json in response, got %s", bodyText)
	}
	if !strings.Contains(bodyText, "Latency (ms):") {
		t.Fatalf("expected latency in response, got %s", bodyText)
	}
	if !strings.Contains(bodyText, "prompt_tokens: 3") {
		t.Fatalf("expected usage metadata in response, got %s", bodyText)
	}
}

func TestTestInterfaceSubmitUpstreamErrorPassThrough(t *testing.T) {
	t.Parallel()

	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"upstream validation failure"}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
		EnableTestUI: true,
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	values := url.Values{}
	values.Set("user_id", "tenant-a")
	values.Set("prompt", "hello")
	resp, err := http.PostForm(gateway.URL+testInterfaceSubmitPath, values)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "Proxy Status: 400") {
		t.Fatalf("expected proxy status 400 in response, got %s", bodyText)
	}
	if !strings.Contains(bodyText, "upstream validation failure") {
		t.Fatalf("expected upstream error payload in response, got %s", bodyText)
	}
}
