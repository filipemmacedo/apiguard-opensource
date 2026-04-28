package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	EnvListenAddr               = "LISTEN_ADDR"
	EnvUpstreamBaseURL          = "UPSTREAM_BASE_URL"
	EnvUpstreamAPIKey           = "UPSTREAM_API_KEY"
	EnvUpstreamTimeout          = "UPSTREAM_TIMEOUT"
	EnvTenantAPIKeys            = "TENANT_API_KEYS"
	EnvEnableTestUI             = "ENABLE_TEST_INTERFACE"
	EnvUsageDBPath              = "USAGE_DB_PATH"
	EnvDatabaseDriver           = "DATABASE_DRIVER"
	EnvDatabaseDSN              = "DATABASE_DSN"
	EnvSecretMasterKey          = "SECRET_MASTER_KEY"
	EnvSecretKeyVer             = "SECRET_KEY_VERSION"
	EnvOpenAIBaseURL            = "OPENAI_BASE_URL"
	EnvLegacyFallback           = "ENABLE_LEGACY_FALLBACK"
)

type Config struct {
	ListenAddr                  string
	UpstreamBaseURL             string
	UpstreamAPIKey              string
	UpstreamTimeout             time.Duration
	TenantByAPIKey              map[string]string
	EnableTestUI                bool
	UsageDBPath                 string
	DatabaseDriver              string
	DatabaseDSN                 string
	SecretMasterKey             string
	SecretKeyVer                string
	OpenAIBaseURL               string
	LegacyFallback              bool
	LegacyFallbackConfigured    bool
}

func LoadFromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("getenv is required")
	}

	cfg := Config{
		ListenAddr:      getenv(EnvListenAddr),
		UpstreamBaseURL: strings.TrimSpace(getenv(EnvUpstreamBaseURL)),
		UpstreamAPIKey:  strings.TrimSpace(getenv(EnvUpstreamAPIKey)),
		UsageDBPath:     strings.TrimSpace(getenv(EnvUsageDBPath)),
		DatabaseDriver:  strings.ToLower(strings.TrimSpace(getenv(EnvDatabaseDriver))),
		DatabaseDSN:     strings.TrimSpace(getenv(EnvDatabaseDSN)),
		SecretMasterKey: strings.TrimSpace(getenv(EnvSecretMasterKey)),
		SecretKeyVer:    strings.TrimSpace(getenv(EnvSecretKeyVer)),
		OpenAIBaseURL:   strings.TrimSpace(getenv(EnvOpenAIBaseURL)),
	}

	enableTestUIRaw := strings.TrimSpace(getenv(EnvEnableTestUI))
	if enableTestUIRaw == "" {
		cfg.EnableTestUI = false
	} else {
		enabled, err := strconv.ParseBool(enableTestUIRaw)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be a boolean: %w", EnvEnableTestUI, err)
		}
		cfg.EnableTestUI = enabled
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.UsageDBPath == "" {
		cfg.UsageDBPath = "usage.db"
	}
	if cfg.SecretKeyVer == "" {
		cfg.SecretKeyVer = "v1"
	}
	if cfg.OpenAIBaseURL == "" {
		if cfg.UpstreamBaseURL != "" {
			cfg.OpenAIBaseURL = cfg.UpstreamBaseURL
		} else {
			cfg.OpenAIBaseURL = "https://api.openai.com"
		}
	}

	timeoutRaw := strings.TrimSpace(getenv(EnvUpstreamTimeout))
	if timeoutRaw == "" {
		cfg.UpstreamTimeout = 30 * time.Second
	} else {
		timeout, err := time.ParseDuration(timeoutRaw)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be a valid duration: %w", EnvUpstreamTimeout, err)
		}
		cfg.UpstreamTimeout = timeout
	}

	tenantMapRaw := strings.TrimSpace(getenv(EnvTenantAPIKeys))
	if tenantMapRaw != "" {
		tenantMap, err := ParseTenantAPIKeys(tenantMapRaw)
		if err != nil {
			return Config{}, err
		}
		cfg.TenantByAPIKey = tenantMap
	}

	legacyFallbackRaw := strings.TrimSpace(getenv(EnvLegacyFallback))
	if legacyFallbackRaw == "" {
		cfg.LegacyFallback = true
	} else {
		enabled, err := strconv.ParseBool(legacyFallbackRaw)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be a boolean: %w", EnvLegacyFallback, err)
		}
		cfg.LegacyFallback = enabled
		cfg.LegacyFallbackConfigured = true
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func ParseTenantAPIKeys(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s is required", EnvTenantAPIKeys)
	}

	tenantByAPIKey := map[string]string{}
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s entry %q must use tenant_id:api_key format", EnvTenantAPIKeys, pair)
		}

		tenantID := strings.TrimSpace(parts[0])
		apiKey := strings.TrimSpace(parts[1])
		if tenantID == "" || apiKey == "" {
			return nil, fmt.Errorf("%s entry %q has empty tenant or api key", EnvTenantAPIKeys, pair)
		}

		if _, exists := tenantByAPIKey[apiKey]; exists {
			return nil, fmt.Errorf("%s contains duplicate api key", EnvTenantAPIKeys)
		}
		tenantByAPIKey[apiKey] = tenantID
	}

	if len(tenantByAPIKey) == 0 {
		return nil, fmt.Errorf("%s must contain at least one tenant_id:api_key pair", EnvTenantAPIKeys)
	}

	return tenantByAPIKey, nil
}

func (c Config) Validate() error {
	if c.UpstreamTimeout <= 0 {
		return fmt.Errorf("%s must be greater than zero", EnvUpstreamTimeout)
	}
	if _, _, err := c.ResolveStorage(); err != nil {
		return err
	}
	hasLegacyBase := strings.TrimSpace(c.UpstreamBaseURL) != ""
	hasLegacyKey := strings.TrimSpace(c.UpstreamAPIKey) != ""
	if hasLegacyBase != hasLegacyKey {
		return fmt.Errorf("%s and %s must be provided together", EnvUpstreamBaseURL, EnvUpstreamAPIKey)
	}
	return nil
}

func (c Config) ResolveStorage() (string, string, error) {
	dsn := strings.TrimSpace(c.DatabaseDSN)

	// Reject legacy SQLite DSNs.
	if strings.HasPrefix(dsn, "file:") || strings.HasSuffix(dsn, ".db") {
		return "", "", fmt.Errorf("SQLite is no longer supported. Use a PostgreSQL DSN for %s", EnvDatabaseDSN)
	}

	if dsn == "" {
		dsn = "postgres://apiguard:apiguard@localhost:5432/apiguard?sslmode=disable"
	}

	return "postgres", dsn, nil
}
