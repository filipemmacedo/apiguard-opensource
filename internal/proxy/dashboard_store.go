package proxy

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxDashboardLogsPerTenant = 200

type dashboardLogRecord struct {
	Timestamp        time.Time                 `json:"timestamp"`
	RequestID        string                    `json:"request_id"`
	Model            string                    `json:"model"`
	Status           int                       `json:"status"`
	LatencyMS        int64                     `json:"latency_ms"`
	PromptTokens     *int64                    `json:"prompt_tokens,omitempty"`
	CompletionTokens *int64                    `json:"completion_tokens,omitempty"`
	TotalTokens      *int64                    `json:"total_tokens,omitempty"`
	EstimatedCostEUR *float64                  `json:"estimated_cost_eur,omitempty"`
	PIISummary       *piiRequestSummary        `json:"pii_summary,omitempty"`
	SecuritySummary  *dashboardSecuritySummary `json:"security_summary,omitempty"`
}

type dashboardUsageTotals struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostEUR float64 `json:"estimated_cost_eur"`
}

type dashboardSecuritySummary struct {
	Labels []string `json:"labels,omitempty"`
}

type dashboardQuery struct {
	From  *time.Time
	To    *time.Time
	Limit int
}

type dashboardStore struct {
	sql *sqlStorage
}

func newDashboardStore(storage *sqlStorage) (*dashboardStore, error) {
	if storage == nil {
		return nil, fmt.Errorf("sql storage is required")
	}

	store := &dashboardStore{sql: storage}
	if err := store.migrate(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *dashboardStore) migrate() error {
	schema := dashboardSchema(s.sql.dialect)
	_, err := s.sql.exec(schema)
	if err != nil {
		return fmt.Errorf("migrate usage db: %w", err)
	}
	if err := s.migrateAddEstimatedCostEUR(); err != nil {
		return fmt.Errorf("migrate add estimated_cost_eur: %w", err)
	}
	return nil
}

func (s *dashboardStore) migrateAddEstimatedCostEUR() error {
	return s.sql.addColumnIfMissing("usage_records", "estimated_cost_eur", "REAL NULL")
}

func dashboardSchema(dialect sqlDialect) string {
	schema := `
CREATE TABLE IF NOT EXISTS usage_records (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  timestamp_utc TEXT NOT NULL,
  request_id TEXT NOT NULL,
  model TEXT NOT NULL,
  status INTEGER NOT NULL,
  latency_ms BIGINT NOT NULL,
  prompt_tokens BIGINT NULL,
  completion_tokens BIGINT NULL,
  total_tokens BIGINT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_records_tenant_time
  ON usage_records(tenant_id, timestamp_utc);
CREATE INDEX IF NOT EXISTS idx_usage_records_tenant_request
  ON usage_records(tenant_id, request_id);

CREATE TABLE IF NOT EXISTS pii_findings (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  timestamp_utc TEXT NOT NULL,
  direction TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  action TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  finding_count BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pii_findings_tenant_time
  ON pii_findings(tenant_id, timestamp_utc);
CREATE INDEX IF NOT EXISTS idx_pii_findings_tenant_request
  ON pii_findings(tenant_id, request_id);

CREATE TABLE IF NOT EXISTS guardrail_outcomes (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  timestamp_utc TEXT NOT NULL,
  guardrail_type TEXT NOT NULL,
  action TEXT NOT NULL,
  matched_policy_id TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_guardrail_outcomes_tenant_time
  ON guardrail_outcomes(tenant_id, timestamp_utc);
CREATE INDEX IF NOT EXISTS idx_guardrail_outcomes_tenant_request
  ON guardrail_outcomes(tenant_id, request_id);
`
	if dialect == sqlDialectSQLite {
		schema = strings.ReplaceAll(schema, "BIGSERIAL PRIMARY KEY", "INTEGER PRIMARY KEY AUTOINCREMENT")
	}
	return schema
}

func (s *dashboardStore) record(tenantID string, record dashboardLogRecord) error {
	if tenantID == "" {
		return nil
	}

	const insert = `
INSERT INTO usage_records (
  tenant_id,
  timestamp_utc,
  request_id,
  model,
  status,
  latency_ms,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  estimated_cost_eur
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

	_, err := s.sql.exec(
		insert,
		tenantID,
		record.Timestamp.UTC().Format(time.RFC3339Nano),
		record.RequestID,
		record.Model,
		record.Status,
		record.LatencyMS,
		record.PromptTokens,
		record.CompletionTokens,
		record.TotalTokens,
		record.EstimatedCostEUR,
	)
	if err != nil {
		return fmt.Errorf("insert usage record: %w", err)
	}
	return nil
}

func (s *dashboardStore) recordPIIFindings(tenantID, requestID string, recordedAt time.Time, findings []piiFindingRecord) error {
	if tenantID == "" || requestID == "" || len(findings) == 0 {
		return nil
	}

	const insert = `
INSERT INTO pii_findings (
  tenant_id,
  request_id,
  timestamp_utc,
  direction,
  entity_type,
  action,
  fingerprint,
  finding_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`

	for _, finding := range findings {
		if finding.Count <= 0 {
			continue
		}
		_, err := s.sql.exec(
			insert,
			tenantID,
			requestID,
			recordedAt.UTC().Format(time.RFC3339Nano),
			finding.Direction,
			finding.EntityType,
			finding.Action,
			finding.Fingerprint,
			finding.Count,
		)
		if err != nil {
			return fmt.Errorf("insert pii finding: %w", err)
		}
	}
	return nil
}

func (s *dashboardStore) recordGuardrailOutcomes(tenantID, requestID string, recordedAt time.Time, outcomes []guardrailOutcomeRecord) error {
	if tenantID == "" || requestID == "" || len(outcomes) == 0 {
		return nil
	}

	const insert = `
INSERT INTO guardrail_outcomes (
  tenant_id,
  request_id,
  timestamp_utc,
  guardrail_type,
  action,
  matched_policy_id
) VALUES (?, ?, ?, ?, ?, ?)
`

	for _, outcome := range outcomes {
		if strings.TrimSpace(outcome.GuardrailType) == "" || strings.TrimSpace(outcome.Action) == "" {
			continue
		}
		_, err := s.sql.exec(
			insert,
			tenantID,
			requestID,
			recordedAt.UTC().Format(time.RFC3339Nano),
			outcome.GuardrailType,
			outcome.Action,
			outcome.MatchedPolicyID,
		)
		if err != nil {
			return fmt.Errorf("insert guardrail outcome: %w", err)
		}
	}
	return nil
}

func (s *dashboardStore) logs(tenantID string, query dashboardQuery) ([]dashboardLogRecord, error) {
	sqlQuery := `
SELECT timestamp_utc, request_id, model, status, latency_ms, prompt_tokens, completion_tokens, total_tokens, estimated_cost_eur
FROM usage_records
WHERE tenant_id = ?
`
	args := []any{tenantID}

	if query.From != nil {
		sqlQuery += " AND timestamp_utc >= ?"
		args = append(args, query.From.UTC().Format(time.RFC3339Nano))
	}
	if query.To != nil {
		sqlQuery += " AND timestamp_utc < ?"
		args = append(args, query.To.UTC().Format(time.RFC3339Nano))
	}

	sqlQuery += " ORDER BY timestamp_utc DESC"
	if query.Limit > 0 {
		sqlQuery += " LIMIT ?"
		args = append(args, query.Limit)
	}

	rows, err := s.sql.query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query usage records: %w", err)
	}
	defer rows.Close()

	out := make([]dashboardLogRecord, 0, maxDashboardLogsPerTenant)
	for rows.Next() {
		var (
			timestampUTC     string
			record           dashboardLogRecord
			promptTokens     sql.NullInt64
			completionTokens sql.NullInt64
			totalTokens      sql.NullInt64
			estimatedCostEUR sql.NullFloat64
		)
		if err := rows.Scan(
			&timestampUTC,
			&record.RequestID,
			&record.Model,
			&record.Status,
			&record.LatencyMS,
			&promptTokens,
			&completionTokens,
			&totalTokens,
			&estimatedCostEUR,
		); err != nil {
			return nil, fmt.Errorf("scan usage record: %w", err)
		}

		parsed, err := time.Parse(time.RFC3339Nano, timestampUTC)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %q: %w", timestampUTC, err)
		}
		record.Timestamp = parsed

		if promptTokens.Valid {
			record.PromptTokens = &promptTokens.Int64
		}
		if completionTokens.Valid {
			record.CompletionTokens = &completionTokens.Int64
		}
		if totalTokens.Valid {
			record.TotalTokens = &totalTokens.Int64
		}
		if estimatedCostEUR.Valid {
			record.EstimatedCostEUR = &estimatedCostEUR.Float64
		}

		out = append(out, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage records: %w", err)
	}

	requestIDs := make([]string, 0, len(out))
	for _, record := range out {
		requestIDs = append(requestIDs, record.RequestID)
	}
	summaries, err := s.requestPIISummaries(tenantID, query, requestIDs)
	if err != nil {
		return nil, err
	}
	guardrailLabels, err := s.requestGuardrailLabels(tenantID, query, requestIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if summary, ok := summaries[out[i].RequestID]; ok {
			out[i].PIISummary = &summary
		}
		if securitySummary := buildRequestSecuritySummary(out[i].PIISummary, guardrailLabels[out[i].RequestID]); securitySummary != nil {
			out[i].SecuritySummary = securitySummary
		}
	}

	return out, nil
}

func (s *dashboardStore) usage(tenantID string, query dashboardQuery) (dashboardUsageTotals, error) {
	sqlQuery := `
SELECT
  COALESCE(SUM(prompt_tokens), 0),
  COALESCE(SUM(completion_tokens), 0),
  COALESCE(SUM(total_tokens), 0),
  COALESCE(SUM(estimated_cost_eur), 0)
FROM usage_records
WHERE tenant_id = ?
`
	args := []any{tenantID}
	if query.From != nil {
		sqlQuery += " AND timestamp_utc >= ?"
		args = append(args, query.From.UTC().Format(time.RFC3339Nano))
	}
	if query.To != nil {
		sqlQuery += " AND timestamp_utc < ?"
		args = append(args, query.To.UTC().Format(time.RFC3339Nano))
	}

	var totals dashboardUsageTotals
	err := s.sql.queryRow(sqlQuery, args...).Scan(
		&totals.PromptTokens,
		&totals.CompletionTokens,
		&totals.TotalTokens,
		&totals.EstimatedCostEUR,
	)
	if err != nil {
		return dashboardUsageTotals{}, fmt.Errorf("query usage totals: %w", err)
	}
	return totals, nil
}

func (s *dashboardStore) piiSummary(tenantID string, query dashboardQuery) (piiTenantSummary, error) {
	rows, err := s.queryPIIFindings(tenantID, query, nil)
	if err != nil {
		return piiTenantSummary{}, err
	}
	defer rows.Close()

	return scanPIITenantSummary(rows)
}

func (s *dashboardStore) securitySummary(tenantID string, query dashboardQuery) (dashboardSecuritySummary, error) {
	labels := map[string]struct{}{}

	piiSummary, err := s.piiSummary(tenantID, query)
	if err != nil {
		return dashboardSecuritySummary{}, err
	}
	if piiSummary.FlaggedRequestCount > 0 {
		labels["PII"] = struct{}{}
	}

	guardrailLabels, err := s.guardrailLabels(tenantID, query)
	if err != nil {
		return dashboardSecuritySummary{}, err
	}
	for _, label := range guardrailLabels {
		labels[label] = struct{}{}
	}

	return dashboardSecuritySummary{Labels: mapKeysSorted(labels)}, nil
}

func (s *dashboardStore) requestPIISummaries(tenantID string, query dashboardQuery, requestIDs []string) (map[string]piiRequestSummary, error) {
	requestIDs = uniqueStrings(requestIDs)
	if len(requestIDs) == 0 {
		return map[string]piiRequestSummary{}, nil
	}
	rows, err := s.queryPIIFindings(tenantID, query, requestIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPIIRequestSummaries(rows)
}

func (s *dashboardStore) queryPIIFindings(tenantID string, query dashboardQuery, requestIDs []string) (*sql.Rows, error) {
	sqlQuery := `
SELECT request_id, timestamp_utc, direction, entity_type, action, finding_count
FROM pii_findings
WHERE tenant_id = ?
`
	args := []any{tenantID}

	if query.From != nil {
		sqlQuery += " AND timestamp_utc >= ?"
		args = append(args, query.From.UTC().Format(time.RFC3339Nano))
	}
	if query.To != nil {
		sqlQuery += " AND timestamp_utc < ?"
		args = append(args, query.To.UTC().Format(time.RFC3339Nano))
	}
	if len(requestIDs) > 0 {
		sqlQuery += " AND request_id IN (" + placeholders(len(requestIDs)) + ")"
		for _, requestID := range requestIDs {
			args = append(args, requestID)
		}
	}
	sqlQuery += " ORDER BY timestamp_utc DESC, request_id DESC"

	rows, err := s.sql.query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query pii findings: %w", err)
	}
	return rows, nil
}

func (s *dashboardStore) requestGuardrailLabels(tenantID string, query dashboardQuery, requestIDs []string) (map[string][]string, error) {
	requestIDs = uniqueStrings(requestIDs)
	if len(requestIDs) == 0 {
		return map[string][]string{}, nil
	}
	rows, err := s.queryGuardrailOutcomes(tenantID, query, requestIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRequestGuardrailLabels(rows)
}

func (s *dashboardStore) guardrailLabels(tenantID string, query dashboardQuery) ([]string, error) {
	rows, err := s.queryGuardrailOutcomes(tenantID, query, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTenantGuardrailLabels(rows)
}

func (s *dashboardStore) queryGuardrailOutcomes(tenantID string, query dashboardQuery, requestIDs []string) (*sql.Rows, error) {
	sqlQuery := `
SELECT request_id, guardrail_type
FROM guardrail_outcomes
WHERE tenant_id = ?
`
	args := []any{tenantID}

	if query.From != nil {
		sqlQuery += " AND timestamp_utc >= ?"
		args = append(args, query.From.UTC().Format(time.RFC3339Nano))
	}
	if query.To != nil {
		sqlQuery += " AND timestamp_utc < ?"
		args = append(args, query.To.UTC().Format(time.RFC3339Nano))
	}
	if len(requestIDs) > 0 {
		sqlQuery += " AND request_id IN (" + placeholders(len(requestIDs)) + ")"
		for _, requestID := range requestIDs {
			args = append(args, requestID)
		}
	}
	sqlQuery += " ORDER BY timestamp_utc DESC, request_id DESC"

	rows, err := s.sql.query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query guardrail outcomes: %w", err)
	}
	return rows, nil
}

func scanPIITenantSummary(rows *sql.Rows) (piiTenantSummary, error) {
	var summary piiTenantSummary
	requests := map[string]struct{}{}
	entityTypes := map[string]struct{}{}
	var lastDetectedAt *string

	for rows.Next() {
		var (
			requestID    string
			timestampUTC string
			direction    string
			entityType   string
			action       string
			count        int64
		)
		if err := rows.Scan(&requestID, &timestampUTC, &direction, &entityType, &action, &count); err != nil {
			return piiTenantSummary{}, fmt.Errorf("scan pii finding: %w", err)
		}
		requests[requestID] = struct{}{}
		entityTypes[entityType] = struct{}{}
		switch direction {
		case piiDirectionIngress:
			summary.IngressFindingCount += count
		case piiDirectionEgress:
			summary.EgressFindingCount += count
		}
		if lastDetectedAt == nil || timestampUTC > *lastDetectedAt {
			value := timestampUTC
			lastDetectedAt = &value
		}
	}
	if err := rows.Err(); err != nil {
		return piiTenantSummary{}, fmt.Errorf("iterate pii findings: %w", err)
	}

	summary.FlaggedRequestCount = int64(len(requests))
	summary.EntityTypes = mapKeysSorted(entityTypes)
	summary.LastDetectedAt = lastDetectedAt
	return summary, nil
}

func scanPIIRequestSummaries(rows *sql.Rows) (map[string]piiRequestSummary, error) {
	summaries := map[string]piiRequestSummary{}
	entityTypesByRequest := map[string]map[string]struct{}{}
	actionsByRequest := map[string]map[string]struct{}{}

	for rows.Next() {
		var (
			requestID    string
			timestampUTC string
			direction    string
			entityType   string
			action       string
			count        int64
		)
		if err := rows.Scan(&requestID, &timestampUTC, &direction, &entityType, &action, &count); err != nil {
			return nil, fmt.Errorf("scan pii request summary: %w", err)
		}
		summary := summaries[requestID]
		switch direction {
		case piiDirectionIngress:
			summary.IngressFindingCount += count
		case piiDirectionEgress:
			summary.EgressFindingCount += count
		}
		summaries[requestID] = summary
		if _, ok := entityTypesByRequest[requestID]; !ok {
			entityTypesByRequest[requestID] = map[string]struct{}{}
		}
		entityTypesByRequest[requestID][entityType] = struct{}{}
		if _, ok := actionsByRequest[requestID]; !ok {
			actionsByRequest[requestID] = map[string]struct{}{}
		}
		actionsByRequest[requestID][action] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pii request summaries: %w", err)
	}

	for requestID, summary := range summaries {
		summary.EntityTypes = mapKeysSorted(entityTypesByRequest[requestID])
		summary.Actions = mapKeysSorted(actionsByRequest[requestID])
		summaries[requestID] = summary
	}
	return summaries, nil
}

func scanRequestGuardrailLabels(rows *sql.Rows) (map[string][]string, error) {
	labelsByRequest := map[string]map[string]struct{}{}

	for rows.Next() {
		var (
			requestID     string
			guardrailType string
		)
		if err := rows.Scan(&requestID, &guardrailType); err != nil {
			return nil, fmt.Errorf("scan guardrail outcome: %w", err)
		}
		label := guardrailTypeLabel(guardrailType)
		if label == "" {
			continue
		}
		if _, ok := labelsByRequest[requestID]; !ok {
			labelsByRequest[requestID] = map[string]struct{}{}
		}
		labelsByRequest[requestID][label] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guardrail outcomes: %w", err)
	}

	out := make(map[string][]string, len(labelsByRequest))
	for requestID, labels := range labelsByRequest {
		out[requestID] = mapKeysSorted(labels)
	}
	return out, nil
}

func scanTenantGuardrailLabels(rows *sql.Rows) ([]string, error) {
	labels := map[string]struct{}{}

	for rows.Next() {
		var (
			requestID     string
			guardrailType string
		)
		if err := rows.Scan(&requestID, &guardrailType); err != nil {
			return nil, fmt.Errorf("scan tenant guardrail outcome: %w", err)
		}
		_ = requestID
		label := guardrailTypeLabel(guardrailType)
		if label == "" {
			continue
		}
		labels[label] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant guardrail outcomes: %w", err)
	}

	return mapKeysSorted(labels), nil
}

func buildRequestSecuritySummary(piiSummary *piiRequestSummary, guardrailLabels []string) *dashboardSecuritySummary {
	labels := map[string]struct{}{}
	if piiSummary != nil && piiSummary.IngressFindingCount+piiSummary.EgressFindingCount > 0 {
		labels["PII"] = struct{}{}
	}
	for _, label := range guardrailLabels {
		labels[label] = struct{}{}
	}
	if len(labels) == 0 {
		return nil
	}
	return &dashboardSecuritySummary{Labels: mapKeysSorted(labels)}
}

func guardrailTypeLabel(guardrailType string) string {
	switch guardrailType {
	case guardrailTypeNSFWKeyword:
		return "NSFW"
	case guardrailTypeRateLimiter:
		return "Rate Limit"
	case guardrailTypeCostLimit:
		return "Cost Limit"
	default:
		if strings.TrimSpace(guardrailType) == "" {
			return ""
		}
		return strings.ToUpper(strings.TrimSpace(guardrailType))
	}
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for i := range values {
		values[i] = "?"
	}
	return strings.Join(values, ", ")
}

func mapKeysSorted(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
