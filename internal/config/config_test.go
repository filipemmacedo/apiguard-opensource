package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvSuccess(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		EnvListenAddr:      ":9090",
		EnvUpstreamBaseURL: "https://upstream.example.com",
		EnvUpstreamAPIKey:  "upstream-key",
		EnvUpstreamTimeout: "5s",
		EnvTenantAPIKeys:   "tenant-a:key-a,tenant-b:key-b",
		EnvEnableTestUI:    "true",
		EnvDatabaseDSN:     "postgres://user:pass@localhost:5432/testdb?sslmode=disable",
		EnvSecretMasterKey: "super-secret-master-key",
		EnvSecretKeyVer:    "v2",
		EnvOpenAIBaseURL:   "https://api.openai.example.com",
		EnvLegacyFallback:  "false",
	}

	cfg, err := LoadFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.ListenAddr != ":9090" {
		t.Fatalf("unexpected ListenAddr: %q", cfg.ListenAddr)
	}
	if cfg.UpstreamTimeout != 5*time.Second {
		t.Fatalf("unexpected UpstreamTimeout: %v", cfg.UpstreamTimeout)
	}
	if got := cfg.TenantByAPIKey["key-a"]; got != "tenant-a" {
		t.Fatalf("unexpected tenant for key-a: %q", got)
	}
	if !cfg.EnableTestUI {
		t.Fatal("expected EnableTestUI to be true")
	}
	driver, dsn, err := cfg.ResolveStorage()
	if err != nil {
		t.Fatalf("ResolveStorage returned error: %v", err)
	}
	if driver != "postgres" {
		t.Fatalf("unexpected storage driver: %q", driver)
	}
	if dsn != env[EnvDatabaseDSN] {
		t.Fatalf("unexpected storage dsn: %q", dsn)
	}
	if cfg.SecretMasterKey != "super-secret-master-key" {
		t.Fatalf("unexpected SecretMasterKey: %q", cfg.SecretMasterKey)
	}
	if cfg.SecretKeyVer != "v2" {
		t.Fatalf("unexpected SecretKeyVer: %q", cfg.SecretKeyVer)
	}
	if cfg.OpenAIBaseURL != "https://api.openai.example.com" {
		t.Fatalf("unexpected OpenAIBaseURL: %q", cfg.OpenAIBaseURL)
	}
	if cfg.LegacyFallback {
		t.Fatal("expected LegacyFallback to be false")
	}
}

func TestLoadFromEnvAllowsManagedOnlyConfiguration(t *testing.T) {
	t.Parallel()

	env := map[string]string{}

	cfg, err := LoadFromEnv(func(key string) string { return env[key] })
	if err == nil {
		if cfg.OpenAIBaseURL != "https://api.openai.com" {
			t.Fatalf("unexpected default OpenAIBaseURL: %q", cfg.OpenAIBaseURL)
		}
		if !cfg.LegacyFallback {
			t.Fatal("expected LegacyFallback to default to true")
		}
		return
	}
	t.Fatalf("expected managed-only configuration to load, got error: %v", err)
}

func TestParseTenantAPIKeysInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseTenantAPIKeys("tenant-a-key-a")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoadFromEnvDefaultTestUIDisabled(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		EnvUpstreamBaseURL: "https://upstream.example.com",
		EnvUpstreamAPIKey:  "upstream-key",
		EnvTenantAPIKeys:   "tenant-a:key-a",
	}

	cfg, err := LoadFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.EnableTestUI {
		t.Fatal("expected EnableTestUI to default to false")
	}
	if cfg.SecretKeyVer != "v1" {
		t.Fatalf("expected default SecretKeyVer, got %q", cfg.SecretKeyVer)
	}
	if cfg.OpenAIBaseURL != "https://upstream.example.com" {
		t.Fatalf("expected OpenAIBaseURL to fall back to legacy upstream base, got %q", cfg.OpenAIBaseURL)
	}
}

func TestLoadFromEnvInvalidTestUIBool(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		EnvUpstreamBaseURL: "https://upstream.example.com",
		EnvUpstreamAPIKey:  "upstream-key",
		EnvTenantAPIKeys:   "tenant-a:key-a",
		EnvEnableTestUI:    "not-a-bool",
	}

	_, err := LoadFromEnv(func(key string) string { return env[key] })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), EnvEnableTestUI) {
		t.Fatalf("expected %s error, got: %v", EnvEnableTestUI, err)
	}
}

func TestLoadFromEnvRejectsPartialLegacyProviderConfig(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		EnvUpstreamBaseURL: "https://upstream.example.com",
	}

	_, err := LoadFromEnv(func(key string) string { return env[key] })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), EnvUpstreamAPIKey) {
		t.Fatalf("expected %s error, got: %v", EnvUpstreamAPIKey, err)
	}
}

func TestLoadFromEnvSupportsPostgresStorage(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		EnvDatabaseDSN: "postgres://postgres:postgres@localhost:5432/apiguard?sslmode=disable",
	}

	cfg, err := LoadFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	driver, dsn, err := cfg.ResolveStorage()
	if err != nil {
		t.Fatalf("ResolveStorage returned error: %v", err)
	}
	if driver != "postgres" {
		t.Fatalf("unexpected storage driver: %q", driver)
	}
	if dsn != env[EnvDatabaseDSN] {
		t.Fatalf("unexpected storage dsn: %q", dsn)
	}
}

func TestLoadFromEnvRejectsSQLiteDSN(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"file:usage.db", "usage.db", "data/mydb.db"} {
		env := map[string]string{EnvDatabaseDSN: dsn}
		_, err := LoadFromEnv(func(key string) string { return env[key] })
		if err == nil {
			t.Fatalf("expected error for SQLite DSN %q, got nil", dsn)
		}
		if !strings.Contains(err.Error(), "SQLite is no longer supported") {
			t.Fatalf("expected SQLite rejection error for DSN %q, got: %v", dsn, err)
		}
	}
}

func TestResolveStorageDefaultsToLocalPostgres(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	driver, dsn, err := cfg.ResolveStorage()
	if err != nil {
		t.Fatalf("ResolveStorage returned error: %v", err)
	}
	if driver != "postgres" {
		t.Fatalf("expected default driver postgres, got %q", driver)
	}
	if !strings.HasPrefix(dsn, "postgres://") {
		t.Fatalf("expected postgres DSN, got %q", dsn)
	}
}
