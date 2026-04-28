package proxy

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestRebindSQLPostgres(t *testing.T) {
	t.Parallel()

	got := rebindSQL(sqlDialectPostgres, "SELECT * FROM items WHERE id = ? AND status = ?")
	want := "SELECT * FROM items WHERE id = $1 AND status = $2"
	if got != want {
		t.Fatalf("unexpected rebound SQL: %q", got)
	}
}

func TestOpenSQLStorageRejectsMissingPostgresDSN(t *testing.T) {
	t.Parallel()

	_, err := openSQLStorage("postgres", "")
	if err == nil {
		t.Fatal("expected missing postgres dsn error, got nil")
	}
	if !strings.Contains(err.Error(), "dsn") {
		t.Fatalf("expected dsn error, got: %v", err)
	}
}

func TestPostgresStorageSmoke(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	storage, err := openSQLStorage("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres storage: %v", err)
	}
	t.Cleanup(func() {
		cleanupPostgresTestTables(t, storage)
		_ = storage.close()
	})

	cleanupPostgresTestTables(t, storage)

	dashboard, err := newDashboardStore(storage)
	if err != nil {
		t.Fatalf("newDashboardStore: %v", err)
	}
	management, err := newManagementStore(storage)
	if err != nil {
		t.Fatalf("newManagementStore: %v", err)
	}

	now := time.Now().UTC()
	keyRecord, err := management.createTenantKey(tenantKeyRecord{
		UserID:      "tenant-a",
		DisplayName: "Tenant A",
		LookupKey:   "lookup-tenant-a",
		KeyFormat:   "legacy",
		SecretHash:  "hash",
		SecretMask:  "mask",
		Status:      tenantKeyStatusActive,
		CreatedBy:   "test",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("createTenantKey: %v", err)
	}
	if keyRecord.ID == 0 {
		t.Fatal("expected tenant key id to be populated")
	}

	if err := dashboard.record("tenant-a", dashboardLogRecord{
		Timestamp:    now,
		RequestID:    "req-1",
		Model:        "gpt-4o-mini",
		Status:       200,
		LatencyMS:    12,
		TotalTokens:  int64Pointer(5),
		PromptTokens: int64Pointer(2),
	}); err != nil {
		t.Fatalf("dashboard.record: %v", err)
	}

	logs, err := dashboard.logs("tenant-a", dashboardQuery{Limit: 10})
	if err != nil {
		t.Fatalf("dashboard.logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	usage, err := dashboard.usage("tenant-a", dashboardQuery{})
	if err != nil {
		t.Fatalf("dashboard.usage: %v", err)
	}
	if usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage totals: %+v", usage)
	}

	if err := dashboard.recordPIIFindings("tenant-a", "req-1", now, []piiFindingRecord{{
		Direction:   piiDirectionIngress,
		EntityType:  "email_address",
		Action:      piiActionObserve,
		Fingerprint: "fingerprint-1",
		Count:       1,
	}}); err != nil {
		t.Fatalf("dashboard.recordPIIFindings: %v", err)
	}

	piiSummary, err := dashboard.piiSummary("tenant-a", dashboardQuery{})
	if err != nil {
		t.Fatalf("dashboard.piiSummary: %v", err)
	}
	if piiSummary.FlaggedRequestCount != 1 || piiSummary.IngressFindingCount != 1 {
		t.Fatalf("unexpected pii summary: %+v", piiSummary)
	}
}

func cleanupPostgresTestTables(t *testing.T, storage *sqlStorage) {
	t.Helper()

	statements := []string{
		"DROP TABLE IF EXISTS audit_events",
		"DROP TABLE IF EXISTS pii_guardrail_policies",
		"DROP TABLE IF EXISTS provider_models",
		"DROP TABLE IF EXISTS provider_credentials",
		"DROP TABLE IF EXISTS tenant_api_keys",
		"DROP TABLE IF EXISTS pii_findings",
		"DROP TABLE IF EXISTS usage_records",
	}
	for _, statement := range statements {
		if _, err := storage.exec(statement); err != nil {
			t.Fatalf("cleanup statement %q failed: %v", statement, err)
		}
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
