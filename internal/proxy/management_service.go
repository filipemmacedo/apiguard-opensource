package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	errProviderNotConfigured = errors.New("provider not configured")
	errProviderSecretLocked  = errors.New("provider secret management unavailable")
	errModelNotEnabled       = errors.New("requested model is not enabled")
)

type adminUserKeyView struct {
	ID                  int64      `json:"id"`
	UserID              string     `json:"user_id"`
	DisplayName         string     `json:"display_name"`
	KeyID               string     `json:"key_id,omitempty"`
	MaskedKey           string     `json:"masked_key"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
	GoorgUserID         *string    `json:"goorg_user_id,omitempty"`
	MonthlyCostLimitEUR *float64   `json:"monthly_cost_limit_eur"`
}

type adminProviderCredentialView struct {
	ID                  int64      `json:"id"`
	ProviderType        string     `json:"provider_type"`
	DisplayName         string     `json:"display_name"`
	BaseURL             string     `json:"base_url"`
	MaskedKey           string     `json:"masked_key"`
	Status              string     `json:"status"`
	LastValidatedAt     *time.Time `json:"last_validated_at,omitempty"`
	LastValidationError string     `json:"last_validation_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type adminProviderModelView struct {
	ID                 int64          `json:"id"`
	ProviderModelID    string         `json:"provider_model_id"`
	DisplayName        string         `json:"display_name"`
	Enabled            bool           `json:"enabled"`
	SyncState          string         `json:"sync_state"`
	LastSyncedAt       time.Time      `json:"last_synced_at"`
	SheetAllowed       bool           `json:"sheet_allowed"`
	SheetLastFetchedAt *time.Time     `json:"sheet_last_fetched_at,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type dashboardPlaygroundModelView struct {
	ProviderModelID     string `json:"provider_model_id"`
	DisplayName         string `json:"display_name"`
	ProviderType        string `json:"provider_type"`
	ProviderDisplayName string `json:"provider_display_name"`
}

type adminPIIPolicyView struct {
	EntityType  string    `json:"entity_type"`
	DisplayName string    `json:"display_name"`
	Enabled     bool      `json:"enabled"`
	Action      string    `json:"action"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type adminNSFWBlockedTermView struct {
	ID        int64     `json:"id"`
	Term      string    `json:"term"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type adminRateLimiterConfigView struct {
	Enabled                   bool      `json:"enabled"`
	RequestLimit              int       `json:"request_limit"`
	WindowSeconds             int       `json:"window_seconds"`
	QuarantineDurationSeconds int       `json:"quarantine_duration_seconds"`
	UpdatedAt                 time.Time `json:"updated_at"`
	UpdatedBy                 string    `json:"updated_by"`
}

type adminQuarantineView struct {
	UserID       string     `json:"user_id"`
	LockedAt     time.Time  `json:"locked_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	LockedReason string     `json:"locked_reason"`
	UnlockedAt   *time.Time `json:"unlocked_at,omitempty"`
	UnlockedBy   string     `json:"unlocked_by,omitempty"`
}

func (s *Server) bootstrapManagedConfig() {
	now := time.Now().UTC()
	for apiKey, userID := range s.cfg.TenantByAPIKey {
		if err := s.management.importLegacyTenantKey(userID, userID, apiKey, "legacy-bootstrap", now); err != nil {
			s.logger.Error("failed to import legacy user key", "user_id", userID, "error", err)
		}
	}

	if strings.TrimSpace(s.cfg.UpstreamBaseURL) == "" || strings.TrimSpace(s.cfg.UpstreamAPIKey) == "" {
		return
	}
	if strings.TrimSpace(s.cfg.SecretMasterKey) == "" {
		s.logger.Warn("legacy provider config present but managed provider import skipped because secret storage is unavailable")
		return
	}

	if _, found, err := s.management.getProviderCredentialByType("openai"); err != nil {
		s.logger.Error("failed checking provider bootstrap state", "error", err)
		return
	} else if found {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.UpstreamTimeout)
	defer cancel()
	lister := newModelLister("openai", s.client)
	models, err := lister.listModels(ctx, s.cfg.UpstreamBaseURL, s.cfg.UpstreamAPIKey)
	if err != nil {
		s.logger.Warn("failed to bootstrap legacy provider credential", "error", err)
		return
	}

	encryptedSecret, err := encryptProviderSecret(s.cfg.SecretMasterKey, s.cfg.UpstreamAPIKey)
	if err != nil {
		s.logger.Error("failed to encrypt legacy provider secret", "error", err)
		return
	}
	record := providerCredentialRecord{
		ProviderType:        "openai",
		DisplayName:         "Imported OpenAI",
		BaseURL:             s.cfg.UpstreamBaseURL,
		SecretCiphertext:    encryptedSecret,
		SecretKeyVersion:    s.cfg.SecretKeyVer,
		SecretMask:          maskProviderSecret(s.cfg.UpstreamAPIKey),
		Status:              providerStatusActive,
		LastValidatedAt:     timePointer(now),
		LastValidationError: "",
		CreatedBy:           "legacy-bootstrap",
		CreatedAt:           now,
		UpdatedBy:           "legacy-bootstrap",
		UpdatedAt:           now,
	}
	record, err = s.management.saveProviderCredential(record)
	if err != nil {
		s.logger.Error("failed to save bootstrapped provider credential", "error", err)
		return
	}
	if _, err := s.management.syncProviderModels(record.ID, record.ProviderType, models, now, true); err != nil {
		s.logger.Error("failed to save bootstrapped provider models", "error", err)
	}
}

func (s *Server) listManagedUserKeys() ([]adminUserKeyView, error) {
	records, err := s.management.listTenantKeys()
	if err != nil {
		return nil, err
	}
	views := make([]adminUserKeyView, 0, len(records))
	for _, record := range records {
		views = append(views, adminUserKeyView{
			ID:                  record.ID,
			UserID:              record.UserID,
			DisplayName:         record.DisplayName,
			KeyID:               record.KeyID,
			MaskedKey:           record.SecretMask,
			Status:              record.Status,
			CreatedAt:           record.CreatedAt,
			RevokedAt:           record.RevokedAt,
			LastUsedAt:          record.LastUsedAt,
			GoorgUserID:         record.GoorgUserID,
			MonthlyCostLimitEUR: record.MonthlyCostLimitEUR,
		})
	}
	return views, nil
}

func (s *Server) createManagedUserKey(displayName, actor string, goorgUserID *uint) (adminUserKeyView, string, error) {
	now := time.Now().UTC()
	userID, err := newManagedUserID()
	if err != nil {
		return adminUserKeyView{}, "", err
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = userID
	}
	material, err := newTenantKeyMaterial()
	if err != nil {
		return adminUserKeyView{}, "", err
	}
	hash, err := hashSecretSegment(material.HashInput)
	if err != nil {
		return adminUserKeyView{}, "", err
	}
	var goorgUserIDStr *string
	if goorgUserID != nil {
		s := fmt.Sprintf("%d", *goorgUserID)
		goorgUserIDStr = &s
	}
	record, err := s.management.createTenantKey(tenantKeyRecord{
		UserID:      userID,
		DisplayName: displayName,
		LookupKey:   material.LookupKey,
		KeyFormat:   material.KeyFormat,
		KeyID:       material.KeyID,
		SecretHash:  hash,
		SecretMask:  material.SecretMask,
		Status:      tenantKeyStatusActive,
		CreatedBy:   actor,
		CreatedAt:   now,
		GoorgUserID: goorgUserIDStr,
	})
	if err != nil {
		return adminUserKeyView{}, "", err
	}
	return adminUserKeyView{
		ID:                  record.ID,
		UserID:              record.UserID,
		DisplayName:         record.DisplayName,
		KeyID:               record.KeyID,
		MaskedKey:           record.SecretMask,
		Status:              record.Status,
		CreatedAt:           record.CreatedAt,
		GoorgUserID:         record.GoorgUserID,
		MonthlyCostLimitEUR: record.MonthlyCostLimitEUR,
	}, material.RawKey, nil
}

func (s *Server) setManagedUserKeyCostLimit(id int64, limitEUR *float64) error {
	return s.management.updateUserKeyCostLimit(id, limitEUR)
}

func newManagedUserID() (string, error) {
	entropy, err := randomBytes(8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("user-%x", entropy), nil
}

func (s *Server) revokeManagedUserKey(id int64, actor string) error {
	return s.management.revokeTenantKey(id, actor, time.Now().UTC())
}

func (s *Server) deleteManagedUserKey(id int64, actor string) error {
	return s.management.deleteTenantKey(id, actor, time.Now().UTC())
}

func (s *Server) authenticateManagedTenantKey(rawKey string) (tenantKeyRecord, bool, error) {
	record, found, err := s.lookupManagedTenantKey(rawKey, true)
	if err != nil || !found {
		return record, found, err
	}
	return record, true, nil
}

func (s *Server) lookupManagedTenantKey(rawKey string, touchLastUsed bool) (tenantKeyRecord, bool, error) {
	presented := parsePresentedTenantKey(strings.TrimSpace(rawKey))
	record, found, err := s.management.findTenantKeyByLookupKey(presented.LookupKey)
	if err != nil {
		return tenantKeyRecord{}, false, err
	}
	if found {
		if record.Status != tenantKeyStatusActive {
			return tenantKeyRecord{}, false, nil
		}
		if !verifySecretSegment(record.SecretHash, presented.HashInput) {
			return tenantKeyRecord{}, false, nil
		}
		if touchLastUsed {
			_ = s.management.updateTenantKeyLastUsed(record.ID, time.Now().UTC())
		}
		return record, true, nil
	}

	if !s.cfg.LegacyFallback {
		return tenantKeyRecord{}, false, nil
	}
	tenantID, ok := s.cfg.TenantByAPIKey[rawKey]
	if !ok {
		return tenantKeyRecord{}, false, nil
	}
	return tenantKeyRecord{
		UserID:      tenantID,
		DisplayName: tenantID,
		Status:      tenantKeyStatusActive,
	}, true, nil
}

func (s *Server) listManagedProviderCredentials() ([]adminProviderCredentialView, error) {
	records, err := s.management.listProviderCredentials()
	if err != nil {
		return nil, err
	}
	views := make([]adminProviderCredentialView, 0, len(records))
	for _, record := range records {
		views = append(views, providerCredentialView(record))
	}
	return views, nil
}

func (s *Server) createManagedProviderCredential(ctx context.Context, providerType, displayName, apiKey, baseURL, actor string) (adminProviderCredentialView, []adminProviderModelView, error) {
	if strings.TrimSpace(s.cfg.SecretMasterKey) == "" {
		return adminProviderCredentialView{}, nil, errProviderSecretLocked
	}
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	if _, ok := providerRegistry[providerType]; !ok {
		return adminProviderCredentialView{}, nil, fmt.Errorf("unsupported provider type %q", providerType)
	}
	// Check for duplicate API key across all existing providers.
	newMask := maskProviderSecret(apiKey)
	existing, err := s.management.listProviderCredentials()
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	for _, cred := range existing {
		if cred.SecretMask == newMask && cred.Status != providerStatusDeleted {
			return adminProviderCredentialView{}, nil, fmt.Errorf("This API key is already in use by provider %q", cred.DisplayName)
		}
	}
	return s.saveValidatedProviderCredential(ctx, providerCredentialRecord{}, providerType, displayName, apiKey, baseURL, actor)
}

func (s *Server) rotateManagedProviderCredential(ctx context.Context, id int64, apiKey, actor string) (adminProviderCredentialView, []adminProviderModelView, error) {
	record, found, err := s.management.getProviderCredentialByID(id)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	if !found || record.Status == providerStatusDeleted {
		return adminProviderCredentialView{}, nil, fmt.Errorf("provider %d not found", id)
	}
	return s.saveValidatedProviderCredential(ctx, record, record.ProviderType, record.DisplayName, apiKey, record.BaseURL, actor)
}

func (s *Server) deleteManagedProviderCredential(id int64, actor string) error {
	return s.management.deleteProviderCredential(id, actor, time.Now().UTC())
}

func (s *Server) listManagedProviderModels(id int64) (adminProviderCredentialView, []adminProviderModelView, error) {
	record, found, err := s.management.getProviderCredentialByID(id)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	if !found || record.Status == providerStatusDeleted {
		return adminProviderCredentialView{}, nil, fmt.Errorf("provider %d not found", id)
	}
	models, err := s.management.listProviderModels(id)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	return providerCredentialView(record), s.providerModelsView(models), nil
}

func (s *Server) refreshManagedProviderModels(ctx context.Context, id int64, actor string) (adminProviderCredentialView, []adminProviderModelView, error) {
	record, found, err := s.management.getProviderCredentialByID(id)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	if !found || record.Status == providerStatusDeleted {
		return adminProviderCredentialView{}, nil, fmt.Errorf("provider %d not found", id)
	}
	if strings.TrimSpace(s.cfg.SecretMasterKey) == "" {
		return adminProviderCredentialView{}, nil, errProviderSecretLocked
	}
	apiKey, err := decryptProviderSecret(s.cfg.SecretMasterKey, record.SecretCiphertext)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}

	now := time.Now().UTC()
	lister := newModelLister(record.ProviderType, s.client)
	models, err := lister.listModels(ctx, record.BaseURL, apiKey)
	if err != nil {
		record.LastValidationError = err.Error()
		record.UpdatedBy = actor
		record.UpdatedAt = now
		if _, saveErr := s.management.saveProviderCredential(record); saveErr != nil {
			return adminProviderCredentialView{}, nil, saveErr
		}
		return adminProviderCredentialView{}, nil, err
	}
	record.Status = providerStatusActive
	record.LastValidatedAt = timePointer(now)
	record.LastValidationError = ""
	record.UpdatedBy = actor
	record.UpdatedAt = now
	record, err = s.management.saveProviderCredential(record)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	synced, err := s.management.syncProviderModels(record.ID, record.ProviderType, models, now, false)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	return providerCredentialView(record), s.providerModelsView(synced), nil
}

var errModelNotSheetAllowed = errors.New("model is not allowed by the pricing sheet")

func (s *Server) updateManagedProviderEnabledModels(id int64, enabledModelIDs []string) (adminProviderCredentialView, []adminProviderModelView, error) {
	record, found, err := s.management.getProviderCredentialByID(id)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	if !found || record.Status == providerStatusDeleted {
		return adminProviderCredentialView{}, nil, fmt.Errorf("provider %d not found", id)
	}

	// Validate all models being enabled are sheet-allowed.
	existing, err := s.management.listProviderModels(id)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	byID := make(map[string]providerModelRecord, len(existing))
	for _, m := range existing {
		byID[m.ProviderModelID] = m
	}
	for _, modelID := range enabledModelIDs {
		m, ok := byID[modelID]
		if ok && !m.SheetAllowed {
			return adminProviderCredentialView{}, nil, fmt.Errorf("%w: %s", errModelNotSheetAllowed, modelID)
		}
	}

	models, err := s.management.updateProviderModelEnabled(id, enabledModelIDs)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	return providerCredentialView(record), s.providerModelsView(models), nil
}

func (s *Server) listDashboardPlaygroundModels() ([]dashboardPlaygroundModelView, error) {
	credentials, err := s.management.listProviderCredentials()
	if err != nil {
		return nil, err
	}

	var views []dashboardPlaygroundModelView
	for _, cred := range credentials {
		if cred.Status != providerStatusActive {
			continue
		}

		models, err := s.management.listProviderModels(cred.ID)
		if err != nil {
			return nil, err
		}

		providerDisplayName := strings.TrimSpace(cred.DisplayName)
		if providerDisplayName == "" {
			providerDisplayName = defaultProviderDisplayName(cred.ProviderType)
		}

		for _, model := range models {
			if !model.Enabled || model.SyncState != modelSyncStateSynced || !model.SheetAllowed {
				continue
			}

			displayName := strings.TrimSpace(model.DisplayName)
			if displayName == "" {
				displayName = model.ProviderModelID
			}

			views = append(views, dashboardPlaygroundModelView{
				ProviderModelID:     model.ProviderModelID,
				DisplayName:         displayName,
				ProviderType:        model.ProviderType,
				ProviderDisplayName: providerDisplayName,
			})
		}
	}

	if views == nil {
		views = []dashboardPlaygroundModelView{}
	}
	return views, nil
}

func (s *Server) resolveManagedProviderForModel(ctx context.Context, model string) (providerCredentialRecord, string, error) {
	credentials, err := s.management.listProviderCredentials()
	if err != nil {
		return providerCredentialRecord{}, "", err
	}

	for _, cred := range credentials {
		if cred.Status != providerStatusActive {
			continue
		}
		models, err := s.management.listProviderModels(cred.ID)
		if err != nil {
			return providerCredentialRecord{}, "", err
		}
		if !providerModelEnabled(models, model) {
			continue
		}
		if strings.TrimSpace(s.cfg.SecretMasterKey) == "" {
			return providerCredentialRecord{}, "", errProviderSecretLocked
		}
		apiKey, err := decryptProviderSecret(s.cfg.SecretMasterKey, cred.SecretCiphertext)
		if err != nil {
			return providerCredentialRecord{}, "", err
		}
		return cred, apiKey, nil
	}
	return providerCredentialRecord{}, "", errModelNotEnabled
}

func (s *Server) listManagedPIIPolicies() ([]adminPIIPolicyView, error) {
	records, err := s.management.listPIIPolicies()
	if err != nil {
		return nil, err
	}
	views := make([]adminPIIPolicyView, 0, len(records))
	for _, record := range records {
		views = append(views, adminPIIPolicyView{
			EntityType:  record.EntityType,
			DisplayName: piiDisplayName(record.EntityType),
			Enabled:     record.Enabled,
			Action:      record.Action,
			UpdatedAt:   record.UpdatedAt,
		})
	}
	return views, nil
}

func (s *Server) updateManagedPIIPolicies(policies []piiPolicyRecord, actor string) ([]adminPIIPolicyView, error) {
	now := time.Now().UTC()
	nextRecords := make([]piiPolicyRecord, 0, len(policies))
	for _, policy := range policies {
		policy.EntityType = strings.TrimSpace(policy.EntityType)
		if !validPIIEntityType(policy.EntityType) {
			return nil, fmt.Errorf("unsupported pii entity type %q", policy.EntityType)
		}
		if !validPIIAction(policy.Action) {
			return nil, fmt.Errorf("unsupported pii action %q", policy.Action)
		}
		policy.Action = strings.TrimSpace(policy.Action)
		policy.UpdatedBy = actor
		policy.UpdatedAt = now
		nextRecords = append(nextRecords, policy)
	}
	if len(nextRecords) == 0 {
		return nil, errors.New("at least one pii policy is required")
	}
	if _, err := s.management.savePIIPolicies(nextRecords); err != nil {
		return nil, err
	}
	return s.listManagedPIIPolicies()
}

func (s *Server) activePIIPolicyByEntityType() (map[string]piiPolicyRecord, error) {
	records, err := s.management.listPIIPolicies()
	if err != nil {
		return nil, err
	}
	policies := make(map[string]piiPolicyRecord, len(records))
	for _, record := range records {
		policies[record.EntityType] = record
	}
	return policies, nil
}

func (s *Server) listManagedNSFWBlockedTerms() ([]adminNSFWBlockedTermView, error) {
	records, err := s.management.listNSFWBlockedTerms()
	if err != nil {
		return nil, err
	}
	views := make([]adminNSFWBlockedTermView, 0, len(records))
	for _, record := range records {
		views = append(views, adminNSFWBlockedTermView{
			ID:        record.ID,
			Term:      record.Term,
			Enabled:   record.Enabled,
			UpdatedAt: record.UpdatedAt,
		})
	}
	return views, nil
}

func (s *Server) createManagedNSFWBlockedTerm(term string, enabled bool, actor string) (adminNSFWBlockedTermView, error) {
	now := time.Now().UTC()
	displayValue, normalizedValue, err := validateNSFWTerm(term)
	if err != nil {
		return adminNSFWBlockedTermView{}, err
	}
	if err := s.ensureUniqueNSFWNormalizedTerm(0, normalizedValue); err != nil {
		return adminNSFWBlockedTermView{}, err
	}

	record, err := s.management.createNSFWBlockedTerm(nsfwBlockedTermRecord{
		Term:           displayValue,
		NormalizedTerm: normalizedValue,
		Enabled:        enabled,
		CreatedBy:      actor,
		CreatedAt:      now,
		UpdatedBy:      actor,
		UpdatedAt:      now,
	})
	if err != nil {
		return adminNSFWBlockedTermView{}, err
	}
	return adminNSFWBlockedTermView{
		ID:        record.ID,
		Term:      record.Term,
		Enabled:   record.Enabled,
		UpdatedAt: record.UpdatedAt,
	}, nil
}

func (s *Server) updateManagedNSFWBlockedTerm(id int64, term string, enabled bool, actor string) (adminNSFWBlockedTermView, error) {
	record, found, err := s.management.getNSFWBlockedTermByID(id)
	if err != nil {
		return adminNSFWBlockedTermView{}, err
	}
	if !found {
		return adminNSFWBlockedTermView{}, errNSFWBlockedTermNotFound
	}

	displayValue, normalizedValue, err := validateNSFWTerm(term)
	if err != nil {
		return adminNSFWBlockedTermView{}, err
	}
	if err := s.ensureUniqueNSFWNormalizedTerm(id, normalizedValue); err != nil {
		return adminNSFWBlockedTermView{}, err
	}

	record.Term = displayValue
	record.NormalizedTerm = normalizedValue
	record.Enabled = enabled
	record.UpdatedBy = actor
	record.UpdatedAt = time.Now().UTC()
	record, err = s.management.updateNSFWBlockedTerm(record)
	if err != nil {
		return adminNSFWBlockedTermView{}, err
	}
	return adminNSFWBlockedTermView{
		ID:        record.ID,
		Term:      record.Term,
		Enabled:   record.Enabled,
		UpdatedAt: record.UpdatedAt,
	}, nil
}

func (s *Server) deleteManagedNSFWBlockedTerm(id int64) error {
	if _, found, err := s.management.getNSFWBlockedTermByID(id); err != nil {
		return err
	} else if !found {
		return errNSFWBlockedTermNotFound
	}
	return s.management.deleteNSFWBlockedTerm(id)
}

func (s *Server) activeManagedNSFWBlockedTerms() ([]nsfwBlockedTermRecord, error) {
	records, err := s.management.listNSFWBlockedTerms()
	if err != nil {
		return nil, err
	}
	out := make([]nsfwBlockedTermRecord, 0, len(records))
	for _, record := range records {
		if record.Enabled {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s *Server) ensureUniqueNSFWNormalizedTerm(id int64, normalizedTerm string) error {
	records, err := s.management.listNSFWBlockedTerms()
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.ID == id {
			continue
		}
		if record.NormalizedTerm == normalizedTerm {
			return nsfwBlockedTermValidationError{message: "duplicate normalized nsfw term"}
		}
	}
	return nil
}

func (s *Server) saveValidatedProviderCredential(ctx context.Context, existing providerCredentialRecord, providerType, displayName, apiKey, baseURL, actor string) (adminProviderCredentialView, []adminProviderModelView, error) {
	if strings.TrimSpace(apiKey) == "" {
		return adminProviderCredentialView{}, nil, errors.New("provider api key is required")
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = defaultProviderDisplayName(providerType)
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURLForProvider(providerType)
		if baseURL == "" {
			baseURL = s.cfg.OpenAIBaseURL
		}
	}

	lister := newModelLister(providerType, s.client)
	models, err := lister.listModels(ctx, baseURL, apiKey)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	encryptedSecret, err := encryptProviderSecret(s.cfg.SecretMasterKey, apiKey)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}

	now := time.Now().UTC()
	if existing.ID == 0 {
		existing.CreatedBy = actor
		existing.CreatedAt = now
	}
	existing.ProviderType = providerType
	existing.DisplayName = displayName
	existing.BaseURL = baseURL
	existing.SecretCiphertext = encryptedSecret
	existing.SecretKeyVersion = s.cfg.SecretKeyVer
	existing.SecretMask = maskProviderSecret(apiKey)
	existing.Status = providerStatusActive
	existing.LastValidatedAt = timePointer(now)
	existing.LastValidationError = ""
	existing.UpdatedBy = actor
	existing.UpdatedAt = now

	saved, err := s.management.saveProviderCredential(existing)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	defaultEnabled := existing.ID == 0 || existing.Status == providerStatusDeleted
	synced, err := s.management.syncProviderModels(saved.ID, saved.ProviderType, models, now, defaultEnabled)
	if err != nil {
		return adminProviderCredentialView{}, nil, err
	}
	return providerCredentialView(saved), s.providerModelsView(synced), nil
}

func providerCredentialView(record providerCredentialRecord) adminProviderCredentialView {
	return adminProviderCredentialView{
		ID:                  record.ID,
		ProviderType:        record.ProviderType,
		DisplayName:         record.DisplayName,
		BaseURL:             record.BaseURL,
		MaskedKey:           record.SecretMask,
		Status:              record.Status,
		LastValidatedAt:     record.LastValidatedAt,
		LastValidationError: record.LastValidationError,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}

func (s *Server) providerModelsView(records []providerModelRecord) []adminProviderModelView {
	views := make([]adminProviderModelView, 0, len(records))
	for _, record := range records {
		var metadata map[string]any
		if strings.TrimSpace(record.MetadataJSON) != "" {
			_ = json.Unmarshal([]byte(record.MetadataJSON), &metadata)
		}
		view := adminProviderModelView{
			ID:              record.ID,
			ProviderModelID: record.ProviderModelID,
			DisplayName:     record.DisplayName,
			Enabled:         record.Enabled,
			SyncState:       record.SyncState,
			LastSyncedAt:    record.LastSyncedAt,
			SheetAllowed:    record.SheetAllowed,
			Metadata:        metadata,
		}
		views = append(views, view)
	}
	return views
}

func providerModelEnabled(models []providerModelRecord, model string) bool {
	for _, item := range models {
		if item.ProviderModelID == model && item.Enabled && item.SyncState == modelSyncStateSynced && item.SheetAllowed {
			return true
		}
	}
	return false
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func buildAuditDetails(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func auditResourceID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func writeAuditEvent(store auditEventRepository, actorID, action, resourceType, resourceID, resourceLabel, status, requestID string, details map[string]string) {
	if store == nil {
		return
	}
	_ = store.appendAuditEvent(auditEventRecord{
		ActorID:       actorID,
		ActorRole:     "admin",
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		ResourceLabel: resourceLabel,
		Status:        status,
		RequestID:     requestID,
		DetailsJSON:   buildAuditDetails(details),
		CreatedAt:     time.Now().UTC(),
	})
}

func defaultProviderDisplayName(providerType string) string {
	providerType = strings.TrimSpace(providerType)
	if providerType == "" {
		return "Provider"
	}
	if len(providerType) == 1 {
		return strings.ToUpper(providerType)
	}
	return strings.ToUpper(providerType[:1]) + providerType[1:]
}

func (s *Server) getManagedRateLimiterConfig() (adminRateLimiterConfigView, error) {
	record, found, err := s.management.getRateLimiterConfig()
	if err != nil {
		return adminRateLimiterConfigView{}, err
	}
	if !found {
		return adminRateLimiterConfigView{
			Enabled:                   false,
			RequestLimit:              rateLimiterDefaultLimit,
			WindowSeconds:             rateLimiterDefaultWindowSecs,
			QuarantineDurationSeconds: rateLimiterDefaultQuarantineSecs,
		}, nil
	}
	return adminRateLimiterConfigView{
		Enabled:                   record.Enabled,
		RequestLimit:              record.RequestLimit,
		WindowSeconds:             record.WindowSeconds,
		QuarantineDurationSeconds: record.QuarantineDurationSeconds,
		UpdatedAt:                 record.UpdatedAt,
		UpdatedBy:                 record.UpdatedBy,
	}, nil
}

func (s *Server) saveManagedRateLimiterConfig(enabled bool, requestLimit, windowSeconds, quarantineDurationSeconds int, actor string) (adminRateLimiterConfigView, error) {
	record := rateLimiterConfigRecord{
		Enabled:                   enabled,
		RequestLimit:              requestLimit,
		WindowSeconds:             windowSeconds,
		QuarantineDurationSeconds: quarantineDurationSeconds,
		UpdatedAt:                 time.Now().UTC(),
		UpdatedBy:                 actor,
	}
	saved, err := s.management.upsertRateLimiterConfig(record)
	if err != nil {
		return adminRateLimiterConfigView{}, err
	}
	return adminRateLimiterConfigView{
		Enabled:                   saved.Enabled,
		RequestLimit:              saved.RequestLimit,
		WindowSeconds:             saved.WindowSeconds,
		QuarantineDurationSeconds: saved.QuarantineDurationSeconds,
		UpdatedAt:                 saved.UpdatedAt,
		UpdatedBy:                 saved.UpdatedBy,
	}, nil
}

func (s *Server) listManagedQuarantines() ([]adminQuarantineView, error) {
	records, err := s.management.listActiveQuarantines(time.Now().UTC())
	if err != nil {
		return nil, err
	}
	out := make([]adminQuarantineView, 0, len(records))
	for _, r := range records {
		view := adminQuarantineView{
			UserID:       r.UserID,
			LockedAt:     r.LockedAt,
			ExpiresAt:    r.ExpiresAt,
			LockedReason: r.LockedReason,
			UnlockedAt:   r.UnlockedAt,
			UnlockedBy:   r.UnlockedBy,
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Server) unlockManagedQuarantine(userID, actor string) error {
	return s.management.unlockQuarantine(userID, time.Now().UTC(), actor)
}
