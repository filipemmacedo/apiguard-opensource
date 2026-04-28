package proxy

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var errQuarantineNotFound = errors.New("no active quarantine found for user")

const (
	tenantKeyStatusActive  = "active"
	tenantKeyStatusRevoked = "revoked"
	tenantKeyStatusDeleted = "deleted"

	providerStatusActive  = "active"
	providerStatusInvalid = "invalid"
	providerStatusRevoked = "revoked"
	providerStatusDeleted = "deleted"

	modelSyncStateSynced  = "synced"
	modelSyncStateMissing = "missing"
)

type tenantKeyRepository interface {
	listTenantKeys() ([]tenantKeyRecord, error)
	createTenantKey(record tenantKeyRecord) (tenantKeyRecord, error)
	findTenantKeyByLookupKey(lookupKey string) (tenantKeyRecord, bool, error)
	findUserKeysByUserID(userID string) ([]tenantKeyRecord, error)
	revokeTenantKey(id int64, actor string, at time.Time) error
	deleteTenantKey(id int64, actor string, at time.Time) error
	updateTenantKeyLastUsed(id int64, usedAt time.Time) error
	importLegacyTenantKey(tenantID, displayName, rawKey, actor string, createdAt time.Time) error
}

type providerCredentialRepository interface {
	listProviderCredentials() ([]providerCredentialRecord, error)
	getProviderCredentialByID(id int64) (providerCredentialRecord, bool, error)
	getProviderCredentialByType(providerType string) (providerCredentialRecord, bool, error)
	getActiveProviderCredential() (providerCredentialRecord, bool, error)
	saveProviderCredential(record providerCredentialRecord) (providerCredentialRecord, error)
	deleteProviderCredential(id int64, actor string, at time.Time) error
	importLegacyProviderCredential(providerType, displayName, baseURL, rawSecret, secretKeyVer, actor string, createdAt time.Time) error
}

type providerModelRepository interface {
	listProviderModels(providerCredentialID int64) ([]providerModelRecord, error)
	syncProviderModels(providerCredentialID int64, providerType string, discovered []discoveredProviderModel, syncedAt time.Time, defaultEnabled bool) ([]providerModelRecord, error)
	updateProviderModelEnabled(providerCredentialID int64, enabledModelIDs []string) ([]providerModelRecord, error)
}

type nsfwBlockedTermRepository interface {
	listNSFWBlockedTerms() ([]nsfwBlockedTermRecord, error)
	getNSFWBlockedTermByID(id int64) (nsfwBlockedTermRecord, bool, error)
	createNSFWBlockedTerm(record nsfwBlockedTermRecord) (nsfwBlockedTermRecord, error)
	updateNSFWBlockedTerm(record nsfwBlockedTermRecord) (nsfwBlockedTermRecord, error)
	deleteNSFWBlockedTerm(id int64) error
}

type auditEventRepository interface {
	appendAuditEvent(event auditEventRecord) error
}

type rateLimiterConfigRepository interface {
	getRateLimiterConfig() (rateLimiterConfigRecord, bool, error)
	upsertRateLimiterConfig(record rateLimiterConfigRecord) (rateLimiterConfigRecord, error)
}

type rateLimitRequestRepository interface {
	insertRateLimitRequest(userID string, requestedAt time.Time) error
	countRequestsInWindow(userID string, windowStart time.Time) (int, error)
	deleteRequestsByUserID(userID string) error
	deleteRequestsOlderThan(cutoff time.Time) error
}

type userQuarantineRepository interface {
	getActiveQuarantine(userID string, now time.Time) (userQuarantineRecord, bool, error)
	createQuarantine(record userQuarantineRecord) (userQuarantineRecord, error)
	listActiveQuarantines(now time.Time) ([]userQuarantineRecord, error)
	unlockQuarantine(userID string, unlockedAt time.Time, unlockedBy string) error
}

type tenantKeyRecord struct {
	ID                  int64
	UserID              string
	DisplayName         string
	LookupKey           string
	KeyFormat           string
	KeyID               string
	SecretHash          string
	SecretMask          string
	Status              string
	CreatedBy           string
	CreatedAt           time.Time
	RevokedBy           string
	RevokedAt           *time.Time
	DeletedBy           string
	DeletedAt           *time.Time
	LastUsedAt          *time.Time
	GoorgUserID         *string
	MonthlyCostLimitEUR *float64
}

type providerCredentialRecord struct {
	ID                  int64
	ProviderType        string
	DisplayName         string
	BaseURL             string
	SecretCiphertext    string
	SecretKeyVersion    string
	SecretMask          string
	Status              string
	LastValidatedAt     *time.Time
	LastValidationError string
	CreatedBy           string
	CreatedAt           time.Time
	UpdatedBy           string
	UpdatedAt           time.Time
}

type providerModelRecord struct {
	ID                   int64
	ProviderCredentialID int64
	ProviderType         string
	ProviderModelID      string
	DisplayName          string
	Enabled              bool
	SyncState            string
	MetadataJSON         string
	LastSyncedAt         time.Time
	SheetAllowed         bool
}

type discoveredProviderModel struct {
	ProviderModelID string
	DisplayName     string
	Metadata        map[string]any
}

type auditEventRecord struct {
	ID            int64
	ActorID       string
	ActorRole     string
	Action        string
	ResourceType  string
	ResourceID    string
	ResourceLabel string
	Status        string
	RequestID     string
	DetailsJSON   string
	CreatedAt     time.Time
}

type rateLimiterConfigRecord struct {
	ID                        int64
	Enabled                   bool
	RequestLimit              int
	WindowSeconds             int
	QuarantineDurationSeconds int
	UpdatedAt                 time.Time
	UpdatedBy                 string
}

type rateLimitRequestRecord struct {
	ID          int64
	UserID      string
	RequestedAt time.Time
}

type userQuarantineRecord struct {
	ID           int64
	UserID       string
	LockedAt     time.Time
	ExpiresAt    time.Time
	LockedReason string
	UnlockedAt   *time.Time
	UnlockedBy   string
}

type managementStore struct {
	sql *sqlStorage
}

func newManagementStore(storage *sqlStorage) (*managementStore, error) {
	if storage == nil {
		return nil, errors.New("sql storage is required")
	}

	store := &managementStore{sql: storage}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	if err := store.ensureDefaultPIIPolicies("system", time.Now().UTC()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *managementStore) migrate() error {
	schema := managementSchema(s.sql.dialect)
	_, err := s.sql.exec(schema)
	if err != nil {
		return fmt.Errorf("migrate management db: %w", err)
	}
	if err := s.migrateDropProviderTypeUnique(); err != nil {
		return fmt.Errorf("migrate drop provider_type unique: %w", err)
	}
	if err := s.migrateAddGoorgUserID(); err != nil {
		return fmt.Errorf("migrate add goorg_user_id: %w", err)
	}
	if err := s.migrateAddMonthlyCostLimit(); err != nil {
		return fmt.Errorf("migrate add monthly_cost_limit_eur: %w", err)
	}
	if err := s.migrateAddSheetsSyncTables(); err != nil {
		return fmt.Errorf("migrate add sheets sync tables: %w", err)
	}
	if err := s.migrateAddSheetAllowed(); err != nil {
		return fmt.Errorf("migrate add sheet_allowed: %w", err)
	}
	if err := s.migrateAddModelPricingEUR(); err != nil {
		return fmt.Errorf("migrate add model_pricing eur columns: %w", err)
	}
	if err := s.migrateSetSheetAllowedTrue(); err != nil {
		return fmt.Errorf("migrate set sheet_allowed true: %w", err)
	}
	return nil
}

func (s *managementStore) migrateAddGoorgUserID() error {
	return s.sql.addColumnIfMissing("tenant_api_keys", "goorg_user_id", "TEXT NULL")
}

func (s *managementStore) migrateAddMonthlyCostLimit() error {
	if err := s.sql.addColumnIfMissing("tenant_api_keys", "monthly_cost_limit_usd", "REAL NULL DEFAULT NULL"); err != nil {
		return err
	}
	if err := s.sql.addColumnIfMissing("tenant_api_keys", "monthly_cost_limit_eur", "REAL NULL DEFAULT NULL"); err != nil {
		return err
	}
	_, err := s.sql.exec(`
UPDATE tenant_api_keys
SET monthly_cost_limit_eur = monthly_cost_limit_usd
WHERE monthly_cost_limit_eur IS NULL AND monthly_cost_limit_usd IS NOT NULL
`)
	return err
}

func (s *managementStore) migrateAddSheetsSyncTables() error {
	_, err := s.sql.exec(`
CREATE TABLE IF NOT EXISTS model_pricing (
  model_id TEXT NOT NULL PRIMARY KEY,
  provider TEXT NOT NULL,
  input_price_per_1m_usd REAL NOT NULL,
  output_price_per_1m_usd REAL NOT NULL,
  input_price_per_1m_eur REAL NOT NULL DEFAULT 0,
  output_price_per_1m_eur REAL NOT NULL DEFAULT 0,
  last_fetched_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sheets_sync_state (
  id INTEGER NOT NULL DEFAULT 1 PRIMARY KEY,
  last_synced_at TEXT NULL,
  sync_status TEXT NOT NULL DEFAULT 'never',
  error_message TEXT NULL
);
`)
	return err
}

func (s *managementStore) migrateAddSheetAllowed() error {
	return s.sql.addColumnIfMissing("provider_models", "sheet_allowed", "BOOLEAN NOT NULL DEFAULT FALSE")
}

func (s *managementStore) migrateSetSheetAllowedTrue() error {
	_, err := s.sql.exec(`UPDATE provider_models SET sheet_allowed = TRUE WHERE sheet_allowed = FALSE`)
	return err
}

func (s *managementStore) migrateAddModelPricingEUR() error {
	if err := s.sql.addColumnIfMissing("model_pricing", "input_price_per_1m_eur", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return s.sql.addColumnIfMissing("model_pricing", "output_price_per_1m_eur", "REAL NOT NULL DEFAULT 0")
}

func (s *managementStore) migrateDropProviderTypeUnique() error {
	if s.sql.dialect == sqlDialectSQLite {
		return nil
	}
	// Drop the legacy unique constraint on provider_type if it exists.
	_, err := s.sql.exec(`
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'provider_credentials_provider_type_key'
      AND conrelid = 'provider_credentials'::regclass
  ) THEN
    ALTER TABLE provider_credentials DROP CONSTRAINT provider_credentials_provider_type_key;
  END IF;
END $$;
`)
	return err
}

func managementSchema(dialect sqlDialect) string {
	schema := `
CREATE TABLE IF NOT EXISTS tenant_api_keys (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  lookup_key TEXT NOT NULL UNIQUE,
  key_format TEXT NOT NULL,
  key_id TEXT NULL,
  secret_hash TEXT NOT NULL,
  secret_mask TEXT NOT NULL,
  status TEXT NOT NULL,
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL,
  revoked_by TEXT NULL,
  revoked_at TEXT NULL,
  deleted_by TEXT NULL,
  deleted_at TEXT NULL,
  last_used_at TEXT NULL,
  goorg_user_id TEXT NULL
);
CREATE INDEX IF NOT EXISTS idx_tenant_api_keys_tenant_status
  ON tenant_api_keys(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_tenant_api_keys_key_id
  ON tenant_api_keys(key_id);

CREATE TABLE IF NOT EXISTS provider_credentials (
  id BIGSERIAL PRIMARY KEY,
  provider_type TEXT NOT NULL,
  display_name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  secret_ciphertext TEXT NOT NULL,
  secret_key_version TEXT NOT NULL,
  secret_mask TEXT NOT NULL,
  status TEXT NOT NULL,
  last_validated_at TEXT NULL,
  last_validation_error TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_provider_credentials_status
  ON provider_credentials(status);

CREATE TABLE IF NOT EXISTS provider_models (
  id BIGSERIAL PRIMARY KEY,
  provider_credential_id BIGINT NOT NULL,
  provider_type TEXT NOT NULL,
  provider_model_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  enabled INTEGER NOT NULL,
  sync_state TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  last_synced_at TEXT NOT NULL,
  UNIQUE(provider_credential_id, provider_model_id),
  FOREIGN KEY(provider_credential_id) REFERENCES provider_credentials(id)
);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider
  ON provider_models(provider_credential_id, enabled);

CREATE TABLE IF NOT EXISTS pii_guardrail_policies (
  entity_type TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL,
  action TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS nsfw_blocked_terms (
  id BIGSERIAL PRIMARY KEY,
  term TEXT NOT NULL,
  normalized_term TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL,
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nsfw_blocked_terms_enabled
  ON nsfw_blocked_terms(enabled);

CREATE TABLE IF NOT EXISTS audit_events (
  id BIGSERIAL PRIMARY KEY,
  actor_id TEXT NOT NULL,
  actor_role TEXT NOT NULL,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  resource_label TEXT NOT NULL,
  status TEXT NOT NULL,
  request_id TEXT NOT NULL,
  details_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at
  ON audit_events(created_at);

CREATE TABLE IF NOT EXISTS rate_limiter_config (
  id BIGSERIAL PRIMARY KEY,
  enabled INTEGER NOT NULL DEFAULT 0,
  request_limit INTEGER NOT NULL DEFAULT 100,
  window_seconds INTEGER NOT NULL DEFAULT 60,
  quarantine_duration_seconds INTEGER NOT NULL DEFAULT 43200,
  updated_at TEXT NOT NULL,
  updated_by TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rate_limit_requests (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL,
  requested_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rate_limit_requests_user_at
  ON rate_limit_requests(user_id, requested_at);

CREATE TABLE IF NOT EXISTS user_quarantine (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL,
  locked_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  locked_reason TEXT NOT NULL DEFAULT '',
  unlocked_at TEXT NULL,
  unlocked_by TEXT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_quarantine_user_active
  ON user_quarantine(user_id, unlocked_at, expires_at);

CREATE TABLE IF NOT EXISTS server_config (
  key TEXT NOT NULL PRIMARY KEY,
  value TEXT NOT NULL
);
`
	if dialect == sqlDialectSQLite {
		schema = strings.ReplaceAll(schema, "BIGSERIAL PRIMARY KEY", "INTEGER PRIMARY KEY AUTOINCREMENT")
	}
	return schema
}

func (s *managementStore) listTenantKeys() ([]tenantKeyRecord, error) {
	rows, err := s.sql.query(`
SELECT id, tenant_id, display_name, lookup_key, key_format, COALESCE(key_id, ''), secret_hash, secret_mask,
       status, created_by, created_at, COALESCE(revoked_by, ''), revoked_at, COALESCE(deleted_by, ''), deleted_at, last_used_at, goorg_user_id, monthly_cost_limit_eur
FROM tenant_api_keys
WHERE status != ?
ORDER BY created_at DESC, id DESC
`, tenantKeyStatusDeleted)
	if err != nil {
		return nil, fmt.Errorf("list tenant keys: %w", err)
	}
	defer rows.Close()

	var out []tenantKeyRecord
	for rows.Next() {
		record, err := scanTenantKeyRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant keys: %w", err)
	}
	return out, nil
}

func (s *managementStore) createTenantKey(record tenantKeyRecord) (tenantKeyRecord, error) {
	const insert = `
INSERT INTO tenant_api_keys (
  tenant_id, display_name, lookup_key, key_format, key_id, secret_hash, secret_mask,
  status, created_by, created_at, goorg_user_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	id, err := s.sql.insertReturningID(
		insert,
		record.UserID,
		record.DisplayName,
		record.LookupKey,
		record.KeyFormat,
		nullableString(record.KeyID),
		record.SecretHash,
		record.SecretMask,
		record.Status,
		record.CreatedBy,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.GoorgUserID,
	)
	if err != nil {
		return tenantKeyRecord{}, fmt.Errorf("insert tenant key: %w", err)
	}
	record.ID = id
	return record, nil
}

func (s *managementStore) importLegacyTenantKey(userID, displayName, rawKey, actor string, createdAt time.Time) error {
	material := importLegacyTenantKey(rawKey)
	if _, found, err := s.findTenantKeyByLookupKey(material.LookupKey); err != nil {
		return err
	} else if found {
		return nil
	}
	hash, err := hashSecretSegment(material.HashInput)
	if err != nil {
		return err
	}
	_, err = s.createTenantKey(tenantKeyRecord{
		UserID:      userID,
		DisplayName: displayName,
		LookupKey:   material.LookupKey,
		KeyFormat:   material.KeyFormat,
		SecretHash:  hash,
		SecretMask:  material.SecretMask,
		Status:      tenantKeyStatusActive,
		CreatedBy:   actor,
		CreatedAt:   createdAt,
	})
	return err
}

func (s *managementStore) findTenantKeyByLookupKey(lookupKey string) (tenantKeyRecord, bool, error) {
	row := s.sql.queryRow(`
SELECT id, tenant_id, display_name, lookup_key, key_format, COALESCE(key_id, ''), secret_hash, secret_mask,
       status, created_by, created_at, COALESCE(revoked_by, ''), revoked_at, COALESCE(deleted_by, ''), deleted_at, last_used_at, goorg_user_id, monthly_cost_limit_eur
FROM tenant_api_keys
WHERE lookup_key = ?
LIMIT 1
`, lookupKey)

	record, err := scanTenantKeyRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tenantKeyRecord{}, false, nil
		}
		return tenantKeyRecord{}, false, err
	}
	return record, true, nil
}

func (s *managementStore) findUserKeysByUserID(userID string) ([]tenantKeyRecord, error) {
	rows, err := s.sql.query(`
SELECT id, tenant_id, display_name, lookup_key, key_format, COALESCE(key_id, ''), secret_hash, secret_mask,
       status, created_by, created_at, COALESCE(revoked_by, ''), revoked_at, COALESCE(deleted_by, ''), deleted_at, last_used_at, goorg_user_id, monthly_cost_limit_eur
FROM tenant_api_keys
WHERE tenant_id = ? AND status = ?
ORDER BY created_at DESC, id DESC
`, userID, tenantKeyStatusActive)
	if err != nil {
		return nil, fmt.Errorf("find user keys by user id: %w", err)
	}
	defer rows.Close()

	var out []tenantKeyRecord
	for rows.Next() {
		record, err := scanTenantKeyRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user keys by user id: %w", err)
	}
	return out, nil
}

func (s *managementStore) findTenantKeyByGoorgUserID(goorgUserID uint) (tenantKeyRecord, bool, error) {
	id := fmt.Sprintf("%d", goorgUserID)
	row := s.sql.queryRow(`
SELECT id, tenant_id, display_name, lookup_key, key_format, COALESCE(key_id, ''), secret_hash, secret_mask,
       status, created_by, created_at, COALESCE(revoked_by, ''), revoked_at, COALESCE(deleted_by, ''), deleted_at, last_used_at, goorg_user_id, monthly_cost_limit_eur
FROM tenant_api_keys
WHERE goorg_user_id = ? AND status = ?
LIMIT 1
`, id, tenantKeyStatusActive)

	record, err := scanTenantKeyRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tenantKeyRecord{}, false, nil
		}
		return tenantKeyRecord{}, false, err
	}
	return record, true, nil
}

func (s *managementStore) setGoorgUserID(keyID int64, goorgUserID uint) error {
	id := fmt.Sprintf("%d", goorgUserID)
	_, err := s.sql.exec(`UPDATE tenant_api_keys SET goorg_user_id = ? WHERE id = ?`, id, keyID)
	if err != nil {
		return fmt.Errorf("set goorg user id: %w", err)
	}
	return nil
}

func (s *managementStore) updateUserKeyCostLimit(id int64, limitEUR *float64) error {
	_, err := s.sql.exec(`UPDATE tenant_api_keys SET monthly_cost_limit_eur = ? WHERE id = ?`, limitEUR, id)
	if err != nil {
		return fmt.Errorf("update user key cost limit: %w", err)
	}
	return nil
}

func (s *managementStore) revokeTenantKey(id int64, actor string, at time.Time) error {
	_, err := s.sql.exec(`
UPDATE tenant_api_keys
SET status = ?, revoked_by = ?, revoked_at = ?
WHERE id = ? AND status = ?
`, tenantKeyStatusRevoked, actor, at.UTC().Format(time.RFC3339Nano), id, tenantKeyStatusActive)
	if err != nil {
		return fmt.Errorf("revoke tenant key: %w", err)
	}
	return nil
}

func (s *managementStore) deleteTenantKey(id int64, actor string, at time.Time) error {
	_, err := s.sql.exec(`
UPDATE tenant_api_keys
SET status = ?, deleted_by = ?, deleted_at = ?
WHERE id = ? AND status != ?
`, tenantKeyStatusDeleted, actor, at.UTC().Format(time.RFC3339Nano), id, tenantKeyStatusDeleted)
	if err != nil {
		return fmt.Errorf("delete tenant key: %w", err)
	}
	return nil
}

func (s *managementStore) updateTenantKeyLastUsed(id int64, usedAt time.Time) error {
	_, err := s.sql.exec(`UPDATE tenant_api_keys SET last_used_at = ? WHERE id = ?`, usedAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update tenant key last used: %w", err)
	}
	return nil
}

func (s *managementStore) listProviderCredentials() ([]providerCredentialRecord, error) {
	rows, err := s.sql.query(`
SELECT id, provider_type, display_name, base_url, secret_ciphertext, secret_key_version, secret_mask, status,
       last_validated_at, last_validation_error, created_by, created_at, updated_by, updated_at
FROM provider_credentials
WHERE status != ?
ORDER BY provider_type ASC
`, providerStatusDeleted)
	if err != nil {
		return nil, fmt.Errorf("list provider credentials: %w", err)
	}
	defer rows.Close()

	var out []providerCredentialRecord
	for rows.Next() {
		record, err := scanProviderCredentialRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider credentials: %w", err)
	}
	return out, nil
}

func (s *managementStore) getProviderCredentialByID(id int64) (providerCredentialRecord, bool, error) {
	row := s.sql.queryRow(`
SELECT id, provider_type, display_name, base_url, secret_ciphertext, secret_key_version, secret_mask, status,
       last_validated_at, last_validation_error, created_by, created_at, updated_by, updated_at
FROM provider_credentials
WHERE id = ?
LIMIT 1
`, id)
	record, err := scanProviderCredentialRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return providerCredentialRecord{}, false, nil
		}
		return providerCredentialRecord{}, false, err
	}
	return record, true, nil
}

func (s *managementStore) getProviderCredentialByType(providerType string) (providerCredentialRecord, bool, error) {
	row := s.sql.queryRow(`
SELECT id, provider_type, display_name, base_url, secret_ciphertext, secret_key_version, secret_mask, status,
       last_validated_at, last_validation_error, created_by, created_at, updated_by, updated_at
FROM provider_credentials
WHERE provider_type = ?
LIMIT 1
`, providerType)
	record, err := scanProviderCredentialRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return providerCredentialRecord{}, false, nil
		}
		return providerCredentialRecord{}, false, err
	}
	return record, true, nil
}

func (s *managementStore) getActiveProviderCredential() (providerCredentialRecord, bool, error) {
	row := s.sql.queryRow(`
SELECT id, provider_type, display_name, base_url, secret_ciphertext, secret_key_version, secret_mask, status,
       last_validated_at, last_validation_error, created_by, created_at, updated_by, updated_at
FROM provider_credentials
WHERE status = ?
ORDER BY updated_at DESC
LIMIT 1
`, providerStatusActive)
	record, err := scanProviderCredentialRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return providerCredentialRecord{}, false, nil
		}
		return providerCredentialRecord{}, false, err
	}
	return record, true, nil
}

func (s *managementStore) saveProviderCredential(record providerCredentialRecord) (providerCredentialRecord, error) {
	if record.ID == 0 {
		id, err := s.sql.insertReturningID(`
INSERT INTO provider_credentials (
  provider_type, display_name, base_url, secret_ciphertext, secret_key_version, secret_mask, status,
  last_validated_at, last_validation_error, created_by, created_at, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
			record.ProviderType,
			record.DisplayName,
			record.BaseURL,
			record.SecretCiphertext,
			record.SecretKeyVersion,
			record.SecretMask,
			record.Status,
			formatNullableTime(record.LastValidatedAt),
			record.LastValidationError,
			record.CreatedBy,
			record.CreatedAt.UTC().Format(time.RFC3339Nano),
			record.UpdatedBy,
			record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return providerCredentialRecord{}, fmt.Errorf("insert provider credential: %w", err)
		}
		record.ID = id
		return record, nil
	}

	_, err := s.sql.exec(`
UPDATE provider_credentials
SET display_name = ?, base_url = ?, secret_ciphertext = ?, secret_key_version = ?, secret_mask = ?, status = ?,
    last_validated_at = ?, last_validation_error = ?, updated_by = ?, updated_at = ?
WHERE id = ?
`,
		record.DisplayName,
		record.BaseURL,
		record.SecretCiphertext,
		record.SecretKeyVersion,
		record.SecretMask,
		record.Status,
		formatNullableTime(record.LastValidatedAt),
		record.LastValidationError,
		record.UpdatedBy,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		record.ID,
	)
	if err != nil {
		return providerCredentialRecord{}, fmt.Errorf("update provider credential: %w", err)
	}
	return record, nil
}

func (s *managementStore) deleteProviderCredential(id int64, actor string, at time.Time) error {
	_, err := s.sql.exec(`
UPDATE provider_credentials
SET status = ?, updated_by = ?, updated_at = ?
WHERE id = ? AND status != ?
`, providerStatusDeleted, actor, at.UTC().Format(time.RFC3339Nano), id, providerStatusDeleted)
	if err != nil {
		return fmt.Errorf("delete provider credential: %w", err)
	}
	_, err = s.sql.exec(`
UPDATE provider_models
SET enabled = 0, sync_state = ?
WHERE provider_credential_id = ?
`, modelSyncStateMissing, id)
	if err != nil {
		return fmt.Errorf("disable provider models on delete: %w", err)
	}
	return nil
}

func (s *managementStore) importLegacyProviderCredential(providerType, displayName, baseURL, rawSecret, secretKeyVer, actor string, createdAt time.Time) error {
	if strings.TrimSpace(rawSecret) == "" || strings.TrimSpace(baseURL) == "" {
		return nil
	}
	if _, found, err := s.getProviderCredentialByType(providerType); err != nil {
		return err
	} else if found {
		return nil
	}
	return errors.New("legacy provider import requires explicit validated save")
}

func (s *managementStore) listProviderModels(providerCredentialID int64) ([]providerModelRecord, error) {
	rows, err := s.sql.query(`
SELECT id, provider_credential_id, provider_type, provider_model_id, display_name, enabled, sync_state, metadata_json, last_synced_at, sheet_allowed
FROM provider_models
WHERE provider_credential_id = ?
ORDER BY display_name ASC, provider_model_id ASC
`, providerCredentialID)
	if err != nil {
		return nil, fmt.Errorf("list provider models: %w", err)
	}
	defer rows.Close()

	var out []providerModelRecord
	for rows.Next() {
		record, err := scanProviderModelRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider models: %w", err)
	}
	return out, nil
}

func (s *managementStore) syncProviderModels(providerCredentialID int64, providerType string, discovered []discoveredProviderModel, syncedAt time.Time, defaultEnabled bool) ([]providerModelRecord, error) {
	tx, err := s.sql.begin()
	if err != nil {
		return nil, fmt.Errorf("begin provider model sync: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.rollback()
		}
	}()

	existingRows, err := tx.query(`
SELECT provider_model_id, enabled
FROM provider_models
WHERE provider_credential_id = ?
`, providerCredentialID)
	if err != nil {
		return nil, fmt.Errorf("list existing provider models: %w", err)
	}
	existingEnabled := map[string]bool{}
	for existingRows.Next() {
		var modelID string
		var enabled int
		if scanErr := existingRows.Scan(&modelID, &enabled); scanErr != nil {
			existingRows.Close()
			return nil, fmt.Errorf("scan existing provider models: %w", scanErr)
		}
		existingEnabled[modelID] = enabled == 1
	}
	existingRows.Close()

	discoveredByID := make(map[string]discoveredProviderModel, len(discovered))
	for _, model := range discovered {
		discoveredByID[model.ProviderModelID] = model
		enabled := defaultEnabled
		if existing, exists := existingEnabled[model.ProviderModelID]; exists {
			enabled = existing
		}
		metadataJSON := "{}"
		if len(model.Metadata) > 0 {
			raw, marshalErr := json.Marshal(model.Metadata)
			if marshalErr != nil {
				return nil, fmt.Errorf("marshal provider model metadata: %w", marshalErr)
			}
			metadataJSON = string(raw)
		}
		if _, execErr := tx.exec(`
INSERT INTO provider_models (
  provider_credential_id, provider_type, provider_model_id, display_name, enabled, sync_state, metadata_json, last_synced_at, sheet_allowed
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, TRUE)
ON CONFLICT(provider_credential_id, provider_model_id)
DO UPDATE SET display_name = excluded.display_name, enabled = excluded.enabled, sync_state = excluded.sync_state,
              metadata_json = excluded.metadata_json, last_synced_at = excluded.last_synced_at, sheet_allowed = TRUE
`,
			providerCredentialID,
			providerType,
			model.ProviderModelID,
			model.DisplayName,
			boolToInt(enabled),
			modelSyncStateSynced,
			metadataJSON,
			syncedAt.UTC().Format(time.RFC3339Nano),
		); execErr != nil {
			return nil, fmt.Errorf("upsert provider model: %w", execErr)
		}
	}

	for existingID := range existingEnabled {
		if _, exists := discoveredByID[existingID]; exists {
			continue
		}
		if _, execErr := tx.exec(`
UPDATE provider_models
SET enabled = 0, sync_state = ?, last_synced_at = ?
WHERE provider_credential_id = ? AND provider_model_id = ?
`, modelSyncStateMissing, syncedAt.UTC().Format(time.RFC3339Nano), providerCredentialID, existingID); execErr != nil {
			return nil, fmt.Errorf("mark missing provider model: %w", execErr)
		}
	}

	if commitErr := tx.commit(); commitErr != nil {
		return nil, fmt.Errorf("commit provider model sync: %w", commitErr)
	}
	return s.listProviderModels(providerCredentialID)
}

func (s *managementStore) updateProviderModelEnabled(providerCredentialID int64, enabledModelIDs []string) ([]providerModelRecord, error) {
	tx, err := s.sql.begin()
	if err != nil {
		return nil, fmt.Errorf("begin provider model update: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.rollback()
		}
	}()

	if _, err = tx.exec(`UPDATE provider_models SET enabled = 0 WHERE provider_credential_id = ?`, providerCredentialID); err != nil {
		return nil, fmt.Errorf("disable provider models: %w", err)
	}
	enabledModelIDs = uniqueStrings(enabledModelIDs)
	for _, modelID := range enabledModelIDs {
		if _, err = tx.exec(`
UPDATE provider_models
SET enabled = 1
WHERE provider_credential_id = ? AND provider_model_id = ? AND sync_state = ?
`, providerCredentialID, modelID, modelSyncStateSynced); err != nil {
			return nil, fmt.Errorf("enable provider model: %w", err)
		}
	}
	if err = tx.commit(); err != nil {
		return nil, fmt.Errorf("commit provider model update: %w", err)
	}
	return s.listProviderModels(providerCredentialID)
}

func (s *managementStore) listPIIPolicies() ([]piiPolicyRecord, error) {
	rows, err := s.sql.query(`
SELECT entity_type, enabled, action, updated_by, updated_at
FROM pii_guardrail_policies
ORDER BY entity_type ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list pii policies: %w", err)
	}
	defer rows.Close()

	var out []piiPolicyRecord
	for rows.Next() {
		record, err := scanPIIPolicyRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pii policies: %w", err)
	}
	return out, nil
}

func (s *managementStore) savePIIPolicies(records []piiPolicyRecord) ([]piiPolicyRecord, error) {
	tx, err := s.sql.begin()
	if err != nil {
		return nil, fmt.Errorf("begin pii policy save: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.rollback()
		}
	}()

	for _, record := range records {
		if _, err = tx.exec(`
INSERT INTO pii_guardrail_policies (
  entity_type, enabled, action, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(entity_type)
DO UPDATE SET enabled = excluded.enabled, action = excluded.action, updated_by = excluded.updated_by, updated_at = excluded.updated_at
`,
			record.EntityType,
			boolToInt(record.Enabled),
			record.Action,
			record.UpdatedBy,
			record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return nil, fmt.Errorf("upsert pii policy: %w", err)
		}
	}

	if err = tx.commit(); err != nil {
		return nil, fmt.Errorf("commit pii policy save: %w", err)
	}
	return s.listPIIPolicies()
}

func (s *managementStore) ensureDefaultPIIPolicies(actor string, at time.Time) error {
	existing, err := s.listPIIPolicies()
	if err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(existing))
	for _, record := range existing {
		seen[record.EntityType] = struct{}{}
	}

	var missing []piiPolicyRecord
	for _, record := range defaultPIIPolicies(actor, at) {
		if _, ok := seen[record.EntityType]; ok {
			continue
		}
		missing = append(missing, record)
	}
	if len(missing) == 0 {
		return nil
	}
	_, err = s.savePIIPolicies(missing)
	return err
}

func (s *managementStore) listNSFWBlockedTerms() ([]nsfwBlockedTermRecord, error) {
	rows, err := s.sql.query(`
SELECT id, term, normalized_term, enabled, created_by, created_at, updated_by, updated_at
FROM nsfw_blocked_terms
ORDER BY term ASC, id ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list nsfw blocked terms: %w", err)
	}
	defer rows.Close()

	var out []nsfwBlockedTermRecord
	for rows.Next() {
		record, err := scanNSFWBlockedTermRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nsfw blocked terms: %w", err)
	}
	return out, nil
}

func (s *managementStore) getNSFWBlockedTermByID(id int64) (nsfwBlockedTermRecord, bool, error) {
	row := s.sql.queryRow(`
SELECT id, term, normalized_term, enabled, created_by, created_at, updated_by, updated_at
FROM nsfw_blocked_terms
WHERE id = ?
LIMIT 1
`, id)
	record, err := scanNSFWBlockedTermRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nsfwBlockedTermRecord{}, false, nil
		}
		return nsfwBlockedTermRecord{}, false, err
	}
	return record, true, nil
}

func (s *managementStore) createNSFWBlockedTerm(record nsfwBlockedTermRecord) (nsfwBlockedTermRecord, error) {
	id, err := s.sql.insertReturningID(`
INSERT INTO nsfw_blocked_terms (
  term, normalized_term, enabled, created_by, created_at, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
`,
		record.Term,
		record.NormalizedTerm,
		boolToInt(record.Enabled),
		record.CreatedBy,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.UpdatedBy,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nsfwBlockedTermRecord{}, fmt.Errorf("insert nsfw blocked term: %w", err)
	}
	record.ID = id
	return record, nil
}

func (s *managementStore) updateNSFWBlockedTerm(record nsfwBlockedTermRecord) (nsfwBlockedTermRecord, error) {
	_, err := s.sql.exec(`
UPDATE nsfw_blocked_terms
SET term = ?, normalized_term = ?, enabled = ?, updated_by = ?, updated_at = ?
WHERE id = ?
`,
		record.Term,
		record.NormalizedTerm,
		boolToInt(record.Enabled),
		record.UpdatedBy,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		record.ID,
	)
	if err != nil {
		return nsfwBlockedTermRecord{}, fmt.Errorf("update nsfw blocked term: %w", err)
	}
	return record, nil
}

func (s *managementStore) deleteNSFWBlockedTerm(id int64) error {
	_, err := s.sql.exec(`DELETE FROM nsfw_blocked_terms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete nsfw blocked term: %w", err)
	}
	return nil
}

func (s *managementStore) appendAuditEvent(event auditEventRecord) error {
	if strings.TrimSpace(event.DetailsJSON) == "" {
		event.DetailsJSON = "{}"
	}
	_, err := s.sql.exec(`
INSERT INTO audit_events (
  actor_id, actor_role, action, resource_type, resource_id, resource_label, status, request_id, details_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		event.ActorID,
		event.ActorRole,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		event.ResourceLabel,
		event.Status,
		event.RequestID,
		event.DetailsJSON,
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func scanTenantKeyRecord(scanner interface{ Scan(dest ...any) error }) (tenantKeyRecord, error) {
	var (
		record           tenantKeyRecord
		createdAt        string
		revokedAt        sql.NullString
		deletedAt        sql.NullString
		lastUsedAt       sql.NullString
		goorgUserID      sql.NullString
		monthlyCostLimit sql.NullFloat64
	)
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.DisplayName,
		&record.LookupKey,
		&record.KeyFormat,
		&record.KeyID,
		&record.SecretHash,
		&record.SecretMask,
		&record.Status,
		&record.CreatedBy,
		&createdAt,
		&record.RevokedBy,
		&revokedAt,
		&record.DeletedBy,
		&deletedAt,
		&lastUsedAt,
		&goorgUserID,
		&monthlyCostLimit,
	); err != nil {
		return tenantKeyRecord{}, err
	}
	if goorgUserID.Valid {
		record.GoorgUserID = &goorgUserID.String
	}
	if monthlyCostLimit.Valid {
		v := monthlyCostLimit.Float64
		record.MonthlyCostLimitEUR = &v
	}
	parsedCreatedAt, err := parseDBTime(createdAt)
	if err != nil {
		return tenantKeyRecord{}, err
	}
	record.CreatedAt = parsedCreatedAt
	if record.RevokedAt, err = parseNullDBTime(revokedAt); err != nil {
		return tenantKeyRecord{}, err
	}
	if record.DeletedAt, err = parseNullDBTime(deletedAt); err != nil {
		return tenantKeyRecord{}, err
	}
	if record.LastUsedAt, err = parseNullDBTime(lastUsedAt); err != nil {
		return tenantKeyRecord{}, err
	}
	return record, nil
}

func scanProviderCredentialRecord(scanner interface{ Scan(dest ...any) error }) (providerCredentialRecord, error) {
	var (
		record          providerCredentialRecord
		lastValidatedAt sql.NullString
		createdAt       string
		updatedAt       string
	)
	if err := scanner.Scan(
		&record.ID,
		&record.ProviderType,
		&record.DisplayName,
		&record.BaseURL,
		&record.SecretCiphertext,
		&record.SecretKeyVersion,
		&record.SecretMask,
		&record.Status,
		&lastValidatedAt,
		&record.LastValidationError,
		&record.CreatedBy,
		&createdAt,
		&record.UpdatedBy,
		&updatedAt,
	); err != nil {
		return providerCredentialRecord{}, err
	}
	var err error
	if record.LastValidatedAt, err = parseNullDBTime(lastValidatedAt); err != nil {
		return providerCredentialRecord{}, err
	}
	if record.CreatedAt, err = parseDBTime(createdAt); err != nil {
		return providerCredentialRecord{}, err
	}
	if record.UpdatedAt, err = parseDBTime(updatedAt); err != nil {
		return providerCredentialRecord{}, err
	}
	return record, nil
}

func scanProviderModelRecord(scanner interface{ Scan(dest ...any) error }) (providerModelRecord, error) {
	var (
		record       providerModelRecord
		enabled      int
		lastSynced   string
		sheetAllowed bool
	)
	if err := scanner.Scan(
		&record.ID,
		&record.ProviderCredentialID,
		&record.ProviderType,
		&record.ProviderModelID,
		&record.DisplayName,
		&enabled,
		&record.SyncState,
		&record.MetadataJSON,
		&lastSynced,
		&sheetAllowed,
	); err != nil {
		return providerModelRecord{}, err
	}
	record.Enabled = enabled == 1
	record.SheetAllowed = sheetAllowed
	parsedLastSynced, err := parseDBTime(lastSynced)
	if err != nil {
		return providerModelRecord{}, err
	}
	record.LastSyncedAt = parsedLastSynced
	return record, nil
}

func scanPIIPolicyRecord(scanner interface{ Scan(dest ...any) error }) (piiPolicyRecord, error) {
	var (
		record    piiPolicyRecord
		enabled   int
		updatedAt string
	)
	if err := scanner.Scan(
		&record.EntityType,
		&enabled,
		&record.Action,
		&record.UpdatedBy,
		&updatedAt,
	); err != nil {
		return piiPolicyRecord{}, err
	}
	record.Enabled = enabled == 1
	parsedUpdatedAt, err := parseDBTime(updatedAt)
	if err != nil {
		return piiPolicyRecord{}, err
	}
	record.UpdatedAt = parsedUpdatedAt
	return record, nil
}

func scanNSFWBlockedTermRecord(scanner interface{ Scan(dest ...any) error }) (nsfwBlockedTermRecord, error) {
	var (
		record    nsfwBlockedTermRecord
		enabled   int
		createdAt string
		updatedAt string
	)
	if err := scanner.Scan(
		&record.ID,
		&record.Term,
		&record.NormalizedTerm,
		&enabled,
		&record.CreatedBy,
		&createdAt,
		&record.UpdatedBy,
		&updatedAt,
	); err != nil {
		return nsfwBlockedTermRecord{}, err
	}
	record.Enabled = enabled == 1
	var err error
	if record.CreatedAt, err = parseDBTime(createdAt); err != nil {
		return nsfwBlockedTermRecord{}, err
	}
	if record.UpdatedAt, err = parseDBTime(updatedAt); err != nil {
		return nsfwBlockedTermRecord{}, err
	}
	return record, nil
}

func parseDBTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse db time %q: %w", raw, err)
	}
	return parsed, nil
}

func parseNullDBTime(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	parsed, err := parseDBTime(raw.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func formatNullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// rateLimiterConfigRepository implementation

func (s *managementStore) getRateLimiterConfig() (rateLimiterConfigRecord, bool, error) {
	row := s.sql.queryRow(`
SELECT id, enabled, request_limit, window_seconds, quarantine_duration_seconds, updated_at, updated_by
FROM rate_limiter_config
ORDER BY id DESC
LIMIT 1
`)
	record, err := scanRateLimiterConfigRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rateLimiterConfigRecord{}, false, nil
		}
		return rateLimiterConfigRecord{}, false, fmt.Errorf("get rate limiter config: %w", err)
	}
	return record, true, nil
}

func (s *managementStore) upsertRateLimiterConfig(record rateLimiterConfigRecord) (rateLimiterConfigRecord, error) {
	_, err := s.sql.exec(`
DELETE FROM rate_limiter_config
`)
	if err != nil {
		return rateLimiterConfigRecord{}, fmt.Errorf("clear rate limiter config: %w", err)
	}
	id, err := s.sql.insertReturningID(`
INSERT INTO rate_limiter_config (
  enabled, request_limit, window_seconds, quarantine_duration_seconds, updated_at, updated_by
) VALUES (?, ?, ?, ?, ?, ?)
`,
		boolToInt(record.Enabled),
		record.RequestLimit,
		record.WindowSeconds,
		record.QuarantineDurationSeconds,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		record.UpdatedBy,
	)
	if err != nil {
		return rateLimiterConfigRecord{}, fmt.Errorf("insert rate limiter config: %w", err)
	}
	record.ID = id
	return record, nil
}

func scanRateLimiterConfigRecord(scanner interface{ Scan(dest ...any) error }) (rateLimiterConfigRecord, error) {
	var (
		record    rateLimiterConfigRecord
		enabled   int
		updatedAt string
	)
	if err := scanner.Scan(
		&record.ID,
		&enabled,
		&record.RequestLimit,
		&record.WindowSeconds,
		&record.QuarantineDurationSeconds,
		&updatedAt,
		&record.UpdatedBy,
	); err != nil {
		return rateLimiterConfigRecord{}, err
	}
	record.Enabled = enabled == 1
	var err error
	if record.UpdatedAt, err = parseDBTime(updatedAt); err != nil {
		return rateLimiterConfigRecord{}, err
	}
	return record, nil
}

// rateLimitRequestRepository implementation

func (s *managementStore) insertRateLimitRequest(userID string, requestedAt time.Time) error {
	_, err := s.sql.exec(`
INSERT INTO rate_limit_requests (user_id, requested_at) VALUES (?, ?)
`, userID, requestedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert rate limit request: %w", err)
	}
	return nil
}

func (s *managementStore) countRequestsInWindow(userID string, windowStart time.Time) (int, error) {
	var count int
	err := s.sql.queryRow(`
SELECT COUNT(*) FROM rate_limit_requests
WHERE user_id = ? AND requested_at >= ?
`, userID, windowStart.UTC().Format(time.RFC3339Nano)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count requests in window: %w", err)
	}
	return count, nil
}

func (s *managementStore) deleteRequestsByUserID(userID string) error {
	_, err := s.sql.exec(`DELETE FROM rate_limit_requests WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete rate limit requests for user: %w", err)
	}
	return nil
}

func (s *managementStore) deleteRequestsOlderThan(cutoff time.Time) error {
	_, err := s.sql.exec(`
DELETE FROM rate_limit_requests WHERE requested_at < ?
`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("prune rate limit requests: %w", err)
	}
	return nil
}

// userQuarantineRepository implementation

func (s *managementStore) getActiveQuarantine(userID string, now time.Time) (userQuarantineRecord, bool, error) {
	nowStr := now.UTC().Format(time.RFC3339Nano)
	row := s.sql.queryRow(`
SELECT id, user_id, locked_at, expires_at, locked_reason, unlocked_at, unlocked_by
FROM user_quarantine
WHERE user_id = ? AND unlocked_at IS NULL AND expires_at > ?
ORDER BY id DESC
LIMIT 1
`, userID, nowStr)
	record, err := scanUserQuarantineRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return userQuarantineRecord{}, false, nil
		}
		return userQuarantineRecord{}, false, fmt.Errorf("get active quarantine: %w", err)
	}
	return record, true, nil
}

func (s *managementStore) createQuarantine(record userQuarantineRecord) (userQuarantineRecord, error) {
	id, err := s.sql.insertReturningID(`
INSERT INTO user_quarantine (user_id, locked_at, expires_at, locked_reason)
VALUES (?, ?, ?, ?)
`,
		record.UserID,
		record.LockedAt.UTC().Format(time.RFC3339Nano),
		record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		record.LockedReason,
	)
	if err != nil {
		return userQuarantineRecord{}, fmt.Errorf("create quarantine: %w", err)
	}
	record.ID = id
	return record, nil
}

func (s *managementStore) listActiveQuarantines(now time.Time) ([]userQuarantineRecord, error) {
	nowStr := now.UTC().Format(time.RFC3339Nano)
	rows, err := s.sql.query(`
SELECT id, user_id, locked_at, expires_at, locked_reason, unlocked_at, unlocked_by
FROM user_quarantine
WHERE unlocked_at IS NULL AND expires_at > ?
ORDER BY locked_at DESC
`, nowStr)
	if err != nil {
		return nil, fmt.Errorf("list active quarantines: %w", err)
	}
	defer rows.Close()

	var out []userQuarantineRecord
	for rows.Next() {
		record, err := scanUserQuarantineRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quarantines: %w", err)
	}
	return out, nil
}

func (s *managementStore) unlockQuarantine(userID string, unlockedAt time.Time, unlockedBy string) error {
	result, err := s.sql.exec(`
UPDATE user_quarantine
SET unlocked_at = ?, unlocked_by = ?
WHERE user_id = ? AND unlocked_at IS NULL AND expires_at > ?
`,
		unlockedAt.UTC().Format(time.RFC3339Nano),
		unlockedBy,
		userID,
		unlockedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("unlock quarantine: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("unlock quarantine rows affected: %w", err)
	}
	if rows == 0 {
		return errQuarantineNotFound
	}
	return nil
}

func scanUserQuarantineRecord(scanner interface{ Scan(dest ...any) error }) (userQuarantineRecord, error) {
	var (
		record     userQuarantineRecord
		lockedAt   string
		expiresAt  string
		unlockedAt sql.NullString
		unlockedBy sql.NullString
	)
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&lockedAt,
		&expiresAt,
		&record.LockedReason,
		&unlockedAt,
		&unlockedBy,
	); err != nil {
		return userQuarantineRecord{}, err
	}
	var err error
	if record.LockedAt, err = parseDBTime(lockedAt); err != nil {
		return userQuarantineRecord{}, err
	}
	if record.ExpiresAt, err = parseDBTime(expiresAt); err != nil {
		return userQuarantineRecord{}, err
	}
	if record.UnlockedAt, err = parseNullDBTime(unlockedAt); err != nil {
		return userQuarantineRecord{}, err
	}
	if unlockedBy.Valid {
		record.UnlockedBy = unlockedBy.String
	}
	return record, nil
}

// getOrCreateMasterKey returns the stored master encryption key, generating and
// persisting a new one if none exists. This allows zero-config startup when
// SECRET_MASTER_KEY is not provided via environment variable.
func (s *managementStore) getOrCreateMasterKey() (string, error) {
	rows, err := s.sql.query(`SELECT value FROM server_config WHERE key = 'master_key'`)
	if err != nil {
		return "", fmt.Errorf("load master key: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return "", fmt.Errorf("scan master key: %w", err)
		}
		return key, nil
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate master key: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate master key: %w", err)
	}
	key := hex.EncodeToString(buf)
	if _, err := s.sql.exec(`INSERT INTO server_config (key, value) VALUES ('master_key', ?)`, key); err != nil {
		return "", fmt.Errorf("persist master key: %w", err)
	}
	return key, nil
}

