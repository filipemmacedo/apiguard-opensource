package proxy

import "context"

type contextKey string

const (
	tenantIDContextKey  contextKey = "tenant_id"
	requestIDContextKey contextKey = "request_id"
	logFieldsContextKey contextKey = "log_fields"
	costLimitContextKey contextKey = "cost_limit_eur"
)

type logFields struct {
	Model             string
	Usage             *Usage
	PIIFindings       []piiFindingRecord
	GuardrailOutcomes []guardrailOutcomeRecord
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDContextKey, tenantID)
}

func TenantIDFromContext(ctx context.Context) string {
	tenantID, _ := ctx.Value(tenantIDContextKey).(string)
	return tenantID
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

// WithCostLimitEUR stores the per-key monthly cost limit (nil = no limit) in context.
func WithCostLimitEUR(ctx context.Context, limitEUR *float64) context.Context {
	return context.WithValue(ctx, costLimitContextKey, limitEUR)
}

// CostLimitEURFromContext retrieves the per-key cost limit. Returns nil if not set.
func CostLimitEURFromContext(ctx context.Context) *float64 {
	v, _ := ctx.Value(costLimitContextKey).(*float64)
	return v
}

// WithCostLimitUSD is kept as a legacy alias for older callers. Values are
// treated as EUR by the current code path.
func WithCostLimitUSD(ctx context.Context, limitUSD *float64) context.Context {
	return WithCostLimitEUR(ctx, limitUSD)
}

// CostLimitUSDFromContext is kept as a legacy alias for older callers.
func CostLimitUSDFromContext(ctx context.Context) *float64 {
	return CostLimitEURFromContext(ctx)
}

func withLogFields(ctx context.Context, fields *logFields) context.Context {
	return context.WithValue(ctx, logFieldsContextKey, fields)
}

func logFieldsFromContext(ctx context.Context) *logFields {
	fields, _ := ctx.Value(logFieldsContextKey).(*logFields)
	return fields
}
