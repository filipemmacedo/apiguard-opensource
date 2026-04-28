package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestPIIGuardrailDoesNotLeakRawValuesToLogsAuditOrFindings(t *testing.T) {
	t.Parallel()

	const (
		requestEmail  = "alice@example.com"
		responsePhone = "+1 (202) 555-0199"
	)

	var logBuffer bytes.Buffer
	upstream := newOpenAIManagedProviderTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","choices":[{"message":{"role":"assistant","content":"Call ` + responsePhone + `"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	})
	defer upstream.Close()

	cfg := withManagedProviderBootstrap(config.Config{
		UpstreamTimeout: 2 * time.Second,
		TenantByAPIKey: map[string]string{
			"tenant-key": "tenant-a",
		},
	}, upstream.URL)
	server := newTestServer(t, cfg, slog.New(slog.NewJSONHandler(&logBuffer, nil)))
	gateway := httptest.NewServer(server.Handler())
	defer gateway.Close()

	policies := defaultPIIPolicies("admin", time.Now().UTC())
	patchPolicies := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		patchPolicies = append(patchPolicies, map[string]any{
			"entity_type": policy.EntityType,
			"enabled":     policy.Enabled,
			"action":      policy.Action,
		})
	}
	patchResp := performAdminJSONRequest(t, gateway.URL, http.MethodPatch, "/pii-policy", map[string]any{
		"policies": patchPolicies,
	})
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from pii policy patch, got %d", patchResp.StatusCode)
	}

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Email me at ` + requestEmail + `"}]}`)
	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tenant-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected proxy status: %d", resp.StatusCode)
	}

	logOutput := logBuffer.String()
	if strings.Contains(logOutput, requestEmail) || strings.Contains(logOutput, responsePhone) {
		t.Fatalf("application logs leaked raw pii: %s", logOutput)
	}

	auditRows, err := server.storage.query(`SELECT details_json FROM audit_events ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	defer auditRows.Close()
	for auditRows.Next() {
		var details string
		if err := auditRows.Scan(&details); err != nil {
			t.Fatalf("scan audit details: %v", err)
		}
		if strings.Contains(details, requestEmail) || strings.Contains(details, responsePhone) {
			t.Fatalf("audit event leaked raw pii: %s", details)
		}
	}

	findingRows, err := server.storage.query(`
SELECT direction, entity_type, action, fingerprint, finding_count
FROM pii_findings
ORDER BY id ASC
`)
	if err != nil {
		t.Fatalf("query pii findings: %v", err)
	}
	defer findingRows.Close()

	var findingCount int
	for findingRows.Next() {
		findingCount++
		var (
			direction   string
			entityType  string
			action      string
			fingerprint string
			count       int64
		)
		if err := findingRows.Scan(&direction, &entityType, &action, &fingerprint, &count); err != nil {
			t.Fatalf("scan pii finding: %v", err)
		}
		if direction == "" || entityType == "" || action == "" || fingerprint == "" {
			t.Fatalf("expected safe pii finding metadata, got direction=%q entity_type=%q action=%q fingerprint=%q", direction, entityType, action, fingerprint)
		}
		if count <= 0 {
			t.Fatalf("expected positive finding count, got %d", count)
		}
		if strings.Contains(direction, requestEmail) ||
			strings.Contains(entityType, requestEmail) ||
			strings.Contains(action, requestEmail) ||
			strings.Contains(fingerprint, requestEmail) ||
			strings.Contains(direction, responsePhone) ||
			strings.Contains(entityType, responsePhone) ||
			strings.Contains(action, responsePhone) ||
			strings.Contains(fingerprint, responsePhone) {
			t.Fatalf("pii finding leaked raw value: direction=%q entity_type=%q action=%q fingerprint=%q", direction, entityType, action, fingerprint)
		}
	}
	if findingCount == 0 {
		t.Fatal("expected persisted pii findings")
	}

	logsResp, err := http.Get(gateway.URL + dashboardLogsPath + "?user_api_key=tenant-key")
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	defer logsResp.Body.Close()
	logsBody, err := io.ReadAll(logsResp.Body)
	if err != nil {
		t.Fatalf("read logs body: %v", err)
	}
	if strings.Contains(string(logsBody), requestEmail) || strings.Contains(string(logsBody), responsePhone) {
		t.Fatalf("dashboard logs leaked raw pii: %s", string(logsBody))
	}
}
