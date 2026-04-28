package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDashboardService struct {
	logsByUser            map[string][]dashboardLogRecord
	usageByUser           map[string]dashboardUsageTotals
	piiSummaryByUser      map[string]piiTenantSummary
	securitySummaryByUser map[string]dashboardSecuritySummary
	logsCalls             []string
	usageCalls            []string
	piiCalls              []string
	securityCalls         []string
}

func (f *fakeDashboardService) record(string, dashboardLogRecord) error {
	return nil
}

func (f *fakeDashboardService) recordPIIFindings(string, string, time.Time, []piiFindingRecord) error {
	return nil
}

func (f *fakeDashboardService) recordGuardrailOutcomes(string, string, time.Time, []guardrailOutcomeRecord) error {
	return nil
}

func (f *fakeDashboardService) logs(tenantID string, query dashboardQuery) ([]dashboardLogRecord, error) {
	f.logsCalls = append(f.logsCalls, tenantID)
	return f.logsByUser[tenantID], nil
}

func (f *fakeDashboardService) usage(tenantID string, query dashboardQuery) (dashboardUsageTotals, error) {
	f.usageCalls = append(f.usageCalls, tenantID)
	return f.usageByUser[tenantID], nil
}

func (f *fakeDashboardService) piiSummary(tenantID string, query dashboardQuery) (piiTenantSummary, error) {
	f.piiCalls = append(f.piiCalls, tenantID)
	return f.piiSummaryByUser[tenantID], nil
}

func (f *fakeDashboardService) securitySummary(tenantID string, query dashboardQuery) (dashboardSecuritySummary, error) {
	f.securityCalls = append(f.securityCalls, tenantID)
	return f.securitySummaryByUser[tenantID], nil
}

func TestHandleDashboardLogsUsesAuthenticatedDashboardIdentity(t *testing.T) {
	t.Parallel()

	dashboard := &fakeDashboardService{
		logsByUser: map[string][]dashboardLogRecord{
			"user-a": {{
				RequestID: "req-a",
				Model:     "gpt-4o-mini",
			}},
		},
	}
	server := &Server{dashboard: dashboard}

	req := httptest.NewRequest(http.MethodGet, dashboardLogsPath, nil)
	req = req.WithContext(withDashboardUserID(req.Context(), "user-a", "mask-a"))
	rec := httptest.NewRecorder()

	server.handleDashboardLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(dashboard.logsCalls) != 1 || dashboard.logsCalls[0] != "user-a" {
		t.Fatalf("expected logs to be scoped to user-a, got %+v", dashboard.logsCalls)
	}

	var payload struct {
		UserID string `json:"user_id"`
		Logs   []struct {
			RequestID string `json:"request_id"`
		} `json:"logs"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.UserID != "user-a" {
		t.Fatalf("expected user-a in payload, got %q", payload.UserID)
	}
	if len(payload.Logs) != 1 || payload.Logs[0].RequestID != "req-a" {
		t.Fatalf("unexpected logs payload: %+v", payload.Logs)
	}
}

func TestHandleDashboardUsageUsesAuthenticatedDashboardIdentity(t *testing.T) {
	t.Parallel()

	dashboard := &fakeDashboardService{
		usageByUser: map[string]dashboardUsageTotals{
			"user-a": {TotalTokens: 5},
		},
		piiSummaryByUser: map[string]piiTenantSummary{
			"user-a": {FlaggedRequestCount: 1},
		},
		securitySummaryByUser: map[string]dashboardSecuritySummary{
			"user-a": {Labels: []string{"PII"}},
		},
	}
	server := &Server{dashboard: dashboard}

	req := httptest.NewRequest(http.MethodGet, dashboardUsagePath, nil)
	req = req.WithContext(withDashboardUserID(req.Context(), "user-a", "mask-a"))
	rec := httptest.NewRecorder()

	server.handleDashboardUsage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(dashboard.usageCalls) != 1 || dashboard.usageCalls[0] != "user-a" {
		t.Fatalf("expected usage to be scoped to user-a, got %+v", dashboard.usageCalls)
	}
	if len(dashboard.piiCalls) != 1 || dashboard.piiCalls[0] != "user-a" {
		t.Fatalf("expected pii summary to be scoped to user-a, got %+v", dashboard.piiCalls)
	}
	if len(dashboard.securityCalls) != 1 || dashboard.securityCalls[0] != "user-a" {
		t.Fatalf("expected security summary to be scoped to user-a, got %+v", dashboard.securityCalls)
	}

	var payload struct {
		UserID string                   `json:"user_id"`
		Usage  dashboardUsageTotals     `json:"usage"`
		PII    piiTenantSummary         `json:"pii_summary"`
		Secure dashboardSecuritySummary `json:"security_summary"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.UserID != "user-a" {
		t.Fatalf("expected user-a in payload, got %q", payload.UserID)
	}
	if payload.Usage.TotalTokens != 5 {
		t.Fatalf("expected total tokens 5, got %+v", payload.Usage)
	}
	if payload.PII.FlaggedRequestCount != 1 {
		t.Fatalf("expected flagged request count 1, got %+v", payload.PII)
	}
	if len(payload.Secure.Labels) != 1 || payload.Secure.Labels[0] != "PII" {
		t.Fatalf("unexpected security summary: %+v", payload.Secure)
	}
}
