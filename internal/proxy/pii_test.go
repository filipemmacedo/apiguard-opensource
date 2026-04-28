package proxy

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestManagementStoreSeedsDefaultPIIPolicies(t *testing.T) {
	t.Parallel()

	storage, err := openSQLStorage("sqlite", filepath.Join(t.TempDir(), "management.db"))
	if err != nil {
		t.Fatalf("open sqlite storage: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.close()
	})

	store, err := newManagementStore(storage)
	if err != nil {
		t.Fatalf("newManagementStore: %v", err)
	}

	policies, err := store.listPIIPolicies()
	if err != nil {
		t.Fatalf("listPIIPolicies: %v", err)
	}

	expectedEntityTypes := supportedPIIEntityTypes()
	if len(policies) != len(expectedEntityTypes) {
		t.Fatalf("expected %d default policies, got %d", len(expectedEntityTypes), len(policies))
	}
	for i, entityType := range expectedEntityTypes {
		if policies[i].EntityType != entityType {
			t.Fatalf("expected entity type %q at index %d, got %q", entityType, i, policies[i].EntityType)
		}
		if !policies[i].Enabled {
			t.Fatalf("expected %q policy to be enabled by default", entityType)
		}
		if policies[i].Action != piiActionObserve {
			t.Fatalf("expected %q policy to default to observe, got %q", entityType, policies[i].Action)
		}
	}
}

func TestUpdateManagedPIIPoliciesRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	_, err := server.updateManagedPIIPolicies([]piiPolicyRecord{{
		EntityType: "ssn",
		Enabled:    true,
		Action:     piiActionObserve,
	}}, "admin")
	if err == nil || !strings.Contains(err.Error(), "unsupported pii entity type") {
		t.Fatalf("expected unsupported entity type error, got %v", err)
	}

	_, err = server.updateManagedPIIPolicies([]piiPolicyRecord{{
		EntityType: "email_address",
		Enabled:    true,
		Action:     "redact",
	}}, "admin")
	if err == nil || !strings.Contains(err.Error(), "unsupported pii action") {
		t.Fatalf("expected unsupported action error, got %v", err)
	}
}

func TestPIIFingerprintDeterministicAndScoped(t *testing.T) {
	t.Parallel()

	keyA := derivePIIFingerprintKey(config.Config{SecretMasterKey: "master-a"})
	keyB := derivePIIFingerprintKey(config.Config{SecretMasterKey: "master-b"})

	emailA := fingerprintPIIValue(keyA, "email_address", "user@example.com")
	if emailA == "" {
		t.Fatal("expected fingerprint to be populated")
	}
	if other := fingerprintPIIValue(keyA, "email_address", "user@example.com"); other != emailA {
		t.Fatalf("expected deterministic fingerprint, got %q and %q", emailA, other)
	}
	if other := fingerprintPIIValue(keyA, "phone_number", "user@example.com"); other == emailA {
		t.Fatal("expected entity type to scope fingerprint values")
	}
	if other := fingerprintPIIValue(keyB, "email_address", "user@example.com"); other == emailA {
		t.Fatal("expected secret material to change fingerprint values")
	}
}
