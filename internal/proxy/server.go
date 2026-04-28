package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"apiguard/internal/config"
)

type Server struct {
	cfg               config.Config
	logger            *slog.Logger
	client            *http.Client
	storage           *sqlStorage
	dashboard         dashboardService
	management        *managementStore
	backgroundCtx     context.Context
	piiFingerprintKey string
}

type dashboardService interface {
	record(tenantID string, record dashboardLogRecord) error
	recordPIIFindings(tenantID, requestID string, recordedAt time.Time, findings []piiFindingRecord) error
	recordGuardrailOutcomes(tenantID, requestID string, recordedAt time.Time, outcomes []guardrailOutcomeRecord) error
	logs(tenantID string, query dashboardQuery) ([]dashboardLogRecord, error)
	usage(tenantID string, query dashboardQuery) (dashboardUsageTotals, error)
	piiSummary(tenantID string, query dashboardQuery) (piiTenantSummary, error)
	securitySummary(tenantID string, query dashboardQuery) (dashboardSecuritySummary, error)
}

const (
	testInterfacePath             = "/internal/test-interface"
	testInterfaceSubmitPath       = "/internal/test-interface/submit"
	dashboardMePath               = "/internal/me"
	dashboardLogsPath             = "/internal/dashboard/logs"
	dashboardUsagePath            = "/internal/dashboard/usage"
	dashboardOverviewLogs         = "/internal/dashboard/overview/logs"
	dashboardOverviewUsage        = "/internal/dashboard/overview/usage"
	dashboardPlaygroundPath       = "/internal/dashboard/playground"
	dashboardPlaygroundModelsPath = "/internal/dashboard/playground/models"
	defaultTestModel              = "gpt-4o-mini"
)

var testInterfaceTemplate = template.Must(template.New("test-interface").Parse(`<!doctype html>
<html>
<head><meta charset="utf-8"><title>API Guard Test Interface</title></head>
<body>
  <h1>API Guard Test Interface</h1>
  <p>Model: {{.Model}}</p>
  <form method="post" action="` + testInterfaceSubmitPath + `">
    <div>
      <label>User ID</label><br />
      <input type="text" name="user_id" value="{{.UserID}}" size="80" />
    </div>
    <div>
      <label>Prompt</label><br />
      <textarea name="prompt" rows="10" cols="100">{{.Prompt}}</textarea>
    </div>
    <div>
      <button type="submit">Send Chat Completion</button>
    </div>
  </form>
  {{if .Error}}
  <p>Error: {{.Error}}</p>
  {{end}}
  {{if .Result}}
  <h2>Result</h2>
  <p>Proxy Status: {{.Result.ProxyStatus}}</p>
  <p>Latency (ms): {{.Result.LatencyMS}}</p>
  {{if .Result.HasUsage}}
  <p>prompt_tokens: {{.Result.PromptTokens}}</p>
  <p>completion_tokens: {{.Result.CompletionTokens}}</p>
  <p>total_tokens: {{.Result.TotalTokens}}</p>
  {{else}}
  <p>Usage: unavailable</p>
  {{end}}
  <h3>Raw JSON Response</h3>
  <pre>{{.Result.RawJSON}}</pre>
  {{end}}
</body>
</html>`))

type webTestPageData struct {
	Model  string
	UserID string
	Prompt string
	Error  string
	Result *webTestResult
}

type webTestResult struct {
	ProxyStatus      int
	LatencyMS        int64
	HasUsage         bool
	PromptTokens     string
	CompletionTokens string
	TotalTokens      string
	RawJSON          string
}

func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.UpstreamTimeout <= 0 {
		cfg.UpstreamTimeout = 30 * time.Second
	}
	if cfg.OpenAIBaseURL == "" {
		if cfg.UpstreamBaseURL != "" {
			cfg.OpenAIBaseURL = cfg.UpstreamBaseURL
		} else {
			cfg.OpenAIBaseURL = "https://api.openai.com"
		}
	}
	if !cfg.LegacyFallbackConfigured {
		cfg.LegacyFallback = true
	}
	driver, dsn, err := cfg.ResolveStorage()
	if err != nil {
		logger.Error("failed to resolve storage configuration", "error", err)
		panic(err)
	}
	storage, err := openSQLStorage(driver, dsn)
	if err != nil {
		logger.Error("failed to initialize persistent storage", "error", err, "storage_driver", driver)
		panic(err)
	}
	store, err := newDashboardStore(storage)
	if err != nil {
		_ = storage.close()
		logger.Error("failed to initialize usage storage", "error", err, "storage_driver", driver)
		panic(err)
	}
	management, err := newManagementStore(storage)
	if err != nil {
		_ = storage.close()
		logger.Error("failed to initialize managed credential storage", "error", err, "storage_driver", driver)
		panic(err)
	}
	client := &http.Client{Timeout: cfg.UpstreamTimeout}
	if strings.TrimSpace(cfg.SecretMasterKey) == "" {
		key, err := management.getOrCreateMasterKey()
		if err != nil {
			_ = storage.close()
			logger.Error("failed to initialize master key", "error", err)
			panic(err)
		}
		cfg.SecretMasterKey = key
		logger.Info("using auto-generated master key stored in database (set SECRET_MASTER_KEY env var to use your own)")
	}

	server := &Server{
		cfg:               cfg,
		logger:            logger,
		client:            client,
		storage:           storage,
		dashboard:         store,
		management:        management,
		piiFingerprintKey: derivePIIFingerprintKey(cfg),
	}
	server.bootstrapManagedConfig()

	return server
}

func (s *Server) Close() error {
	if s.storage == nil {
		return nil
	}
	return s.storage.close()
}

func (s *Server) Start(ctx context.Context) {
	s.backgroundCtx = ctx
}

func (s *Server) syncContext() context.Context {
	if s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", s.authMiddleware(http.HandlerFunc(s.handleChatCompletions)))
	mux.Handle(adminAPIManagementPath, s.adminMiddleware(http.HandlerFunc(s.handleAdminAPIManagement)))
	mux.Handle(adminAPIManagementPath+"/", s.adminMiddleware(http.HandlerFunc(s.handleAdminAPIManagement)))
	mux.Handle(dashboardMePath, s.dashboardMiddleware(http.HandlerFunc(s.handleDashboardMe)))
	mux.Handle(dashboardLogsPath, s.dashboardMiddleware(http.HandlerFunc(s.handleDashboardLogs)))
	mux.Handle(dashboardUsagePath, s.dashboardMiddleware(http.HandlerFunc(s.handleDashboardUsage)))
	mux.Handle(dashboardOverviewLogs, s.adminMiddleware(http.HandlerFunc(s.handleDashboardOverviewLogs)))
	mux.Handle(dashboardOverviewUsage, s.adminMiddleware(http.HandlerFunc(s.handleDashboardOverviewUsage)))
	mux.Handle(dashboardPlaygroundPath, s.adminMiddleware(http.HandlerFunc(s.handleDashboardPlayground)))
	mux.Handle(dashboardPlaygroundModelsPath, s.adminMiddleware(http.HandlerFunc(s.handleDashboardPlaygroundModels)))
	if s.cfg.EnableTestUI {
		mux.HandleFunc(testInterfacePath, s.handleTestInterfacePage)
		mux.HandleFunc(testInterfaceSubmitPath, s.handleTestInterfaceSubmit)
	}
	return mux
}

func (s *Server) handleTestInterfacePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.renderTestInterfacePage(w, http.StatusOK, webTestPageData{
		Model: defaultTestModel,
	})
}

func (s *Server) handleTestInterfaceSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderTestInterfacePage(w, http.StatusBadRequest, webTestPageData{
			Model: defaultTestModel,
			Error: "failed to parse form input",
		})
		return
	}

	userID := strings.TrimSpace(r.FormValue("user_id"))
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	pageData := webTestPageData{
		Model:  defaultTestModel,
		UserID: userID,
		Prompt: prompt,
	}
	if userID == "" || prompt == "" {
		pageData.Error = "user id and prompt are required"
		s.renderTestInterfacePage(w, http.StatusBadRequest, pageData)
		return
	}

	activeUserKeys, err := s.management.findUserKeysByUserID(userID)
	if err != nil || len(activeUserKeys) == 0 {
		pageData.Error = "no active API key found for user"
		s.renderTestInterfacePage(w, http.StatusUnauthorized, pageData)
		return
	}
	if err := s.management.updateTenantKeyLastUsed(activeUserKeys[0].ID, time.Now().UTC()); err != nil {
		s.logger.Warn("failed to update last used for test interface key", "user_id", userID, "error", err)
	}

	requestPayload, err := json.Marshal(map[string]any{
		"model": defaultTestModel,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	})
	if err != nil {
		pageData.Error = "failed to build request payload"
		s.renderTestInterfacePage(w, http.StatusInternalServerError, pageData)
		return
	}

	testCtx := WithCostLimitEUR(r.Context(), activeUserKeys[0].MonthlyCostLimitEUR)

	start := time.Now()
	proxyStatus, proxyBody, err := s.executePlaygroundChatCompletion(testCtx, userID, requestPayload)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		pageData.Error = "failed to execute proxy request"
		s.renderTestInterfacePage(w, http.StatusInternalServerError, pageData)
		return
	}

	usage := ExtractUsage(proxyBody)
	pageData.Result = &webTestResult{
		ProxyStatus:      proxyStatus,
		LatencyMS:        latencyMS,
		HasUsage:         usage != nil,
		PromptTokens:     usageTokenString(usage, func(u *Usage) *int64 { return u.PromptTokens }),
		CompletionTokens: usageTokenString(usage, func(u *Usage) *int64 { return u.CompletionTokens }),
		TotalTokens:      usageTokenString(usage, func(u *Usage) *int64 { return u.TotalTokens }),
		RawJSON:          prettyJSON(proxyBody),
	}
	s.renderTestInterfacePage(w, proxyStatus, pageData)
}

func (s *Server) executeProxyChatCompletion(ctx context.Context, userAPIKey string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	capture := &responseCapture{}
	s.authMiddleware(http.HandlerFunc(s.handleChatCompletions)).ServeHTTP(capture, req)
	return capture.statusCode, capture.body.Bytes(), nil
}

// executePlaygroundChatCompletion runs a chat completion for playground-style flows by
// injecting the user ID directly into context after the caller has verified an active key.
// These requests still feed the normal dashboard log/usage persistence path.
func (s *Server) executePlaygroundChatCompletion(ctx context.Context, userID string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	requestID := requestIDFromHeaderOrGenerate("")
	logState := &logFields{}
	req = req.WithContext(WithRequestID(req.Context(), requestID))
	req = req.WithContext(withLogFields(req.Context(), logState))
	req = req.WithContext(WithTenantID(req.Context(), userID))

	start := time.Now()
	capture := &responseCapture{}
	s.handleChatCompletions(capture, req)
	s.logRequest(
		requestID,
		userID,
		logState.Model,
		capture.statusCode,
		time.Since(start),
		logState.Usage,
		logState.PIIFindings,
		logState.GuardrailOutcomes,
	)
	return capture.statusCode, capture.body.Bytes(), nil
}

func (s *Server) renderTestInterfacePage(w http.ResponseWriter, statusCode int, data webTestPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := testInterfaceTemplate.Execute(w, data); err != nil {
		s.logger.Error("failed to render test interface", "error", err)
	}
}

func (s *Server) handleDashboardMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	identity := dashboardIdentityFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":   identity.UserID,
		"key_alias": identity.KeyAlias,
	})
}

func (s *Server) handleDashboardLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := dashboardUserIDFromContext(r.Context())

	query, err := dashboardQueryFromRequest(r, maxDashboardLogsPerTenant)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	logs, err := s.dashboard.logs(userID, query)
	if err != nil {
		s.logger.Error("failed to load dashboard logs", "error", err, "user_id", userID)
		writeJSONError(w, http.StatusInternalServerError, "failed to load logs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": userID,
		"logs":    logs,
	})
}

func (s *Server) handleDashboardUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := dashboardUserIDFromContext(r.Context())

	query, err := dashboardQueryFromRequest(r, 0)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	usage, err := s.dashboard.usage(userID, query)
	if err != nil {
		s.logger.Error("failed to load dashboard usage", "error", err, "user_id", userID)
		writeJSONError(w, http.StatusInternalServerError, "failed to load usage")
		return
	}
	piiSummary, err := s.dashboard.piiSummary(userID, query)
	if err != nil {
		s.logger.Error("failed to load dashboard pii summary", "error", err, "user_id", userID)
		writeJSONError(w, http.StatusInternalServerError, "failed to load usage")
		return
	}
	securitySummary, err := s.dashboard.securitySummary(userID, query)
	if err != nil {
		s.logger.Error("failed to load dashboard security summary", "error", err, "user_id", userID)
		writeJSONError(w, http.StatusInternalServerError, "failed to load usage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":          userID,
		"usage":            usage,
		"pii_summary":      piiSummary,
		"security_summary": securitySummary,
	})
}

func (s *Server) handleDashboardPlayground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload struct {
		UserID string `json:"user_id"`
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json request")
		return
	}

	userID := strings.TrimSpace(payload.UserID)
	if userID == "" {
		writeJSONError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	activeUserKeys, err := s.management.findUserKeysByUserID(userID)
	if err != nil || len(activeUserKeys) == 0 {
		writeJSONError(w, http.StatusUnauthorized, "no active API key found for user")
		return
	}
	if err := s.management.updateTenantKeyLastUsed(activeUserKeys[0].ID, time.Now().UTC()); err != nil {
		s.logger.Warn("failed to update last used for playground key", "user_id", userID, "error", err)
	}

	model := strings.TrimSpace(payload.Model)
	prompt := strings.TrimSpace(payload.Prompt)
	if model == "" || prompt == "" {
		writeJSONError(w, http.StatusBadRequest, "model and prompt are required")
		return
	}

	requestPayload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to build request payload")
		return
	}

	playgroundCtx := WithCostLimitEUR(r.Context(), activeUserKeys[0].MonthlyCostLimitEUR)

	start := time.Now()
	proxyStatus, proxyBody, err := s.executePlaygroundChatCompletion(playgroundCtx, userID, requestPayload)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to execute proxy request")
		return
	}

	writeJSON(w, proxyStatus, map[string]any{
		"proxy_status": proxyStatus,
		"latency_ms":   latencyMS,
		"usage":        ExtractUsage(proxyBody),
		"raw_json":     rawJSONEnvelope(proxyBody),
	})
}

func (s *Server) handleDashboardPlaygroundModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	models, err := s.listDashboardPlaygroundModels()
	if err != nil {
		s.logger.Error("failed to load dashboard playground models", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load playground models")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models": models,
	})
}

func (s *Server) handleDashboardOverviewLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query, err := dashboardQueryFromRequest(r, maxDashboardLogsPerTenant)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	users := s.configuredUsers()
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		logs, err := s.dashboard.logs(user.ID, query)
		if err != nil {
			s.logger.Error("failed to load overview logs", "error", err, "user_id", user.ID)
			writeJSONError(w, http.StatusInternalServerError, "failed to load logs")
			return
		}
		items = append(items, map[string]any{
			"user_id":    user.ID,
			"key_alias":  user.KeyAlias,
			"log_count":  len(logs),
			"logs":       logs,
			"time_range": dashboardRangeResponse(query),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"users": items,
	})
}

func (s *Server) handleDashboardOverviewUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query, err := dashboardQueryFromRequest(r, 0)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	users := s.configuredUsers()
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		usage, err := s.dashboard.usage(user.ID, query)
		if err != nil {
			s.logger.Error("failed to load overview usage", "error", err, "user_id", user.ID)
			writeJSONError(w, http.StatusInternalServerError, "failed to load usage")
			return
		}
		piiSummary, err := s.dashboard.piiSummary(user.ID, query)
		if err != nil {
			s.logger.Error("failed to load overview pii summary", "error", err, "user_id", user.ID)
			writeJSONError(w, http.StatusInternalServerError, "failed to load usage")
			return
		}
		securitySummary, err := s.dashboard.securitySummary(user.ID, query)
		if err != nil {
			s.logger.Error("failed to load overview security summary", "error", err, "user_id", user.ID)
			writeJSONError(w, http.StatusInternalServerError, "failed to load usage")
			return
		}
		items = append(items, map[string]any{
			"user_id":          user.ID,
			"key_alias":        user.KeyAlias,
			"usage":            usage,
			"pii_summary":      piiSummary,
			"security_summary": securitySummary,
			"time_range":       dashboardRangeResponse(query),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"users": items,
	})
}

type dashboardUserIDContextKey struct{}

type dashboardIdentity struct {
	UserID   string
	KeyAlias string
}

func withDashboardUserID(ctx context.Context, userID, keyAlias string) context.Context {
	return context.WithValue(ctx, dashboardUserIDContextKey{}, dashboardIdentity{UserID: userID, KeyAlias: keyAlias})
}

func dashboardUserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(dashboardUserIDContextKey{}).(dashboardIdentity)
	return v.UserID
}

func dashboardIdentityFromContext(ctx context.Context) dashboardIdentity {
	v, _ := ctx.Value(dashboardUserIDContextKey{}).(dashboardIdentity)
	return v
}

// dashboardMiddleware resolves a tenant user ID from the request.
// It accepts an optional `user_id` query parameter; if absent it uses the first
// configured tenant key so the dashboard works without authentication.
func (s *Server) dashboardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		var keyAlias string

		if userID == "" {
			keys, err := s.management.listTenantKeys()
			if err != nil || len(keys) == 0 {
				writeJSONError(w, http.StatusServiceUnavailable, "no tenant keys configured")
				return
			}
			userID = keys[0].UserID
			keyAlias = keys[0].SecretMask
		}

		ctx := withDashboardUserID(r.Context(), userID, keyAlias)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := requestIDFromHeaderOrGenerate(r.Header.Get("X-Request-ID"))
		logState := &logFields{}

		r = r.WithContext(WithRequestID(r.Context(), requestID))
		r = r.WithContext(withLogFields(r.Context(), logState))

		apiKey, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			s.logRequest(requestID, "", "", http.StatusUnauthorized, time.Since(start), nil, nil, nil)
			return
		}

		record, found, err := s.lookupManagedTenantKey(apiKey, true)
		if err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, "user auth unavailable")
			s.logRequest(requestID, "", "", http.StatusServiceUnavailable, time.Since(start), nil, nil, nil)
			return
		}
		if !found {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			s.logRequest(requestID, "", "", http.StatusUnauthorized, time.Since(start), nil, nil, nil)
			return
		}

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		ctx := WithTenantID(r.Context(), record.UserID)
		ctx = WithCostLimitEUR(ctx, record.MonthlyCostLimitEUR)
		next.ServeHTTP(recorder, r.WithContext(ctx))

		s.logRequest(
			requestID,
			record.UserID,
			logState.Model,
			recorder.statusCode,
			time.Since(start),
			logState.Usage,
			logState.PIIFindings,
			logState.GuardrailOutcomes,
		)
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	model, err := validateRequestBody(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if fields := logFieldsFromContext(r.Context()); fields != nil {
		fields.Model = model
	}

	userID := TenantIDFromContext(r.Context())
	if userID != "" {
		costResult, err := s.checkMonthlyCostLimit(userID, r.Context(), time.Now().UTC())
		if err != nil {
			s.logger.Warn("cost limit check failed", "user_id", userID, "error", err)
		} else if costResult.Blocked {
			if fields := logFieldsFromContext(r.Context()); fields != nil {
				fields.GuardrailOutcomes = append(fields.GuardrailOutcomes, guardrailOutcomeRecord{
					GuardrailType: guardrailTypeCostLimit,
					Action:        rateLimiterActionBlock,
				})
			}
			writeJSONError(w, http.StatusTooManyRequests, "request blocked: monthly cost limit exceeded")
			return
		}
	}
	if userID != "" {
		rateLimitResult, err := s.checkRateLimit(userID, time.Now().UTC())
		if err != nil {
			s.logger.Warn("rate limiter check failed", "user_id", userID, "error", err)
		} else if !rateLimitResult.Allowed {
			action := rateLimiterActionBlock
			if rateLimitResult.Quarantine {
				action = rateLimiterActionQuarantine
				writeAuditEvent(
					s.management,
					userID,
					"rate_limiter.quarantine",
					"user",
					userID,
					userID,
					"triggered",
					RequestIDFromContext(r.Context()),
					map[string]string{"expires_at": rateLimitResult.RetryAfter.UTC().Format(time.RFC3339)},
				)
			}
			if fields := logFieldsFromContext(r.Context()); fields != nil {
				fields.GuardrailOutcomes = append(fields.GuardrailOutcomes, guardrailOutcomeRecord{
					GuardrailType: guardrailTypeRateLimiter,
					Action:        action,
				})
			}
			w.Header().Set("Retry-After", rateLimitResult.RetryAfter.UTC().Format(time.RFC1123))
			writeJSONError(w, http.StatusTooManyRequests, "request blocked: rate limit exceeded")
			return
		}
	}

	nsfwTerms, err := s.activeManagedNSFWBlockedTerms()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load nsfw policy")
		return
	}
	nsfwMatches := detectNSFWBlockedTermMatches(extractRequestTextValues(body), nsfwTerms)
	if len(nsfwMatches) > 0 {
		outcomes := nsfwGuardrailOutcomes(nsfwMatches)
		if fields := logFieldsFromContext(r.Context()); fields != nil {
			fields.GuardrailOutcomes = append(fields.GuardrailOutcomes, outcomes...)
		}
		writeJSONError(w, http.StatusBadRequest, "request blocked by content policy")
		return
	}

	policies, err := s.activePIIPolicyByEntityType()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load pii policy")
		return
	}
	ingressFindings := detectPIIFindings(extractRequestPIITexts(body), policies, piiDirectionIngress, s.piiFingerprintKey)
	if fields := logFieldsFromContext(r.Context()); fields != nil {
		fields.PIIFindings = append(fields.PIIFindings, ingressFindings...)
	}
	if hasBlockingPIIFinding(ingressFindings) {
		writeJSONError(w, http.StatusBadRequest, "request blocked by pii policy")
		return
	}

	provider, providerAPIKey, err := s.resolveManagedProviderForModel(r.Context(), model)
	if err != nil {
		switch {
		case errors.Is(err, errModelNotEnabled):
			writeJSONError(w, http.StatusBadRequest, "model is not enabled")
		case errors.Is(err, errProviderNotConfigured), errors.Is(err, errProviderSecretLocked):
			writeJSONError(w, http.StatusServiceUnavailable, "provider configuration unavailable")
		default:
			s.logger.Error("resolve provider failed", "error", err, "model", model)
			writeJSONError(w, http.StatusInternalServerError, "failed to resolve provider configuration")
		}
		return
	}

	upstreamURL := strings.TrimRight(provider.BaseURL, "/") + "/v1/chat/completions"
	upstreamCtx, cancel := context.WithTimeout(r.Context(), s.cfg.UpstreamTimeout)
	defer cancel()

	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to build upstream request")
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+providerAPIKey)

	upstreamResp, err := s.client.Do(upstreamReq)
	if err != nil {
		if isTimeoutError(err) {
			writeJSONError(w, http.StatusGatewayTimeout, "upstream timeout")
			return
		}
		writeJSONError(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	defer upstreamResp.Body.Close()

	upstreamBody, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	if fields := logFieldsFromContext(r.Context()); fields != nil {
		fields.Usage = ExtractUsage(upstreamBody)
		fields.PIIFindings = append(fields.PIIFindings, detectPIIFindings(extractResponsePIITexts(upstreamBody), policies, piiDirectionEgress, s.piiFingerprintKey)...)
	}

	if contentType := upstreamResp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(upstreamResp.StatusCode)
	_, _ = w.Write(upstreamBody)
}

func (s *Server) logRequest(requestID, userID, model string, statusCode int, latency time.Duration, usage *Usage, piiFindings []piiFindingRecord, guardrailOutcomes []guardrailOutcomeRecord) {
	var promptTokens any
	var completionTokens any
	var totalTokens any
	var promptTokensPtr *int64
	var completionTokensPtr *int64
	var totalTokensPtr *int64

	if usage != nil {
		promptTokensPtr = usage.PromptTokens
		completionTokensPtr = usage.CompletionTokens
		totalTokensPtr = usage.TotalTokens
	}
	if promptTokensPtr != nil {
		promptTokens = *promptTokensPtr
	}
	if completionTokensPtr != nil {
		completionTokens = *completionTokensPtr
	}
	if totalTokensPtr != nil {
		totalTokens = *totalTokensPtr
	}
	piiIngressCount, piiEgressCount, piiEntityTypes := piiFindingLogFields(piiFindings)
	guardrailTypes, guardrailActions, guardrailPolicyIDs := guardrailOutcomeLogFields(guardrailOutcomes)

	s.logger.Info("chat_completion_request",
		"request_id", requestID,
		"user_id", userID,
		"model", model,
		"status", statusCode,
		"latency_ms", latency.Milliseconds(),
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
		"total_tokens", totalTokens,
		"pii_ingress_findings", piiIngressCount,
		"pii_egress_findings", piiEgressCount,
		"pii_entity_types", piiEntityTypes,
		"guardrail_types", guardrailTypes,
		"guardrail_actions", guardrailActions,
		"guardrail_matched_policy_ids", guardrailPolicyIDs,
	)

	var costEUR *float64
	if usage != nil {
		var prompt, completion, total int64
		if promptTokensPtr != nil {
			prompt = *promptTokensPtr
		}
		if completionTokensPtr != nil {
			completion = *completionTokensPtr
		}
		if totalTokensPtr != nil {
			total = *totalTokensPtr
		}
		c := s.estimateRequestCost(model, prompt, completion, total)
		costEUR = &c
	} else if statusCode < 400 {
		// Upstream was called but returned no usage data — store 0 so the record
		// is distinguishable from a blocked request (which stores NULL).
		zero := 0.0
		costEUR = &zero
	}
	// statusCode >= 400 with nil usage: request was blocked before the upstream
	// call (cost limit, rate limit, model not enabled, etc.) — cost stays NULL.

	recordedAt := time.Now().UTC()
	if err := s.dashboard.record(userID, dashboardLogRecord{
		Timestamp:        recordedAt,
		RequestID:        requestID,
		Model:            model,
		Status:           statusCode,
		LatencyMS:        latency.Milliseconds(),
		PromptTokens:     promptTokensPtr,
		CompletionTokens: completionTokensPtr,
		TotalTokens:      totalTokensPtr,
		EstimatedCostEUR: costEUR,
	}); err != nil {
		s.logger.Error("failed to persist dashboard usage record",
			"error", err,
			"request_id", requestID,
			"user_id", userID,
		)
	}
	if err := s.dashboard.recordPIIFindings(userID, requestID, recordedAt, piiFindings); err != nil {
		s.logger.Error("failed to persist dashboard pii findings",
			"error", err,
			"request_id", requestID,
			"user_id", userID,
		)
	}
	if err := s.dashboard.recordGuardrailOutcomes(userID, requestID, recordedAt, guardrailOutcomes); err != nil {
		s.logger.Error("failed to persist dashboard guardrail outcomes",
			"error", err,
			"request_id", requestID,
			"user_id", userID,
		)
	}
}

func validateRequestBody(body []byte) (string, error) {
	var payload struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", errors.New("invalid json request")
	}
	if strings.TrimSpace(payload.Model) == "" {
		return "", errors.New("model is required")
	}
	if len(payload.Messages) == 0 {
		return "", errors.New("messages is required")
	}
	return payload.Model, nil
}

func bearerToken(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}

func requestIDFromHeaderOrGenerate(headerValue string) string {
	if strings.TrimSpace(headerValue) != "" {
		return strings.TrimSpace(headerValue)
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "req-fallback"
	}
	return hex.EncodeToString(buf)
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (s *statusRecorder) WriteHeader(statusCode int) {
	s.statusCode = statusCode
	s.ResponseWriter.WriteHeader(statusCode)
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func rawJSONEnvelope(body []byte) any {
	if json.Valid(body) {
		var payload any
		if err := json.Unmarshal(body, &payload); err == nil {
			return payload
		}
	}
	return map[string]string{"raw_text": string(body)}
}

type responseCapture struct {
	headers    http.Header
	statusCode int
	body       bytes.Buffer
}

func (c *responseCapture) Header() http.Header {
	if c.headers == nil {
		c.headers = make(http.Header)
	}
	return c.headers
}

func (c *responseCapture) WriteHeader(statusCode int) {
	c.statusCode = statusCode
}

func (c *responseCapture) Write(p []byte) (int, error) {
	if c.statusCode == 0 {
		c.statusCode = http.StatusOK
	}
	return c.body.Write(p)
}

func usageTokenString(usage *Usage, selectToken func(*Usage) *int64) string {
	if usage == nil {
		return ""
	}
	token := selectToken(usage)
	if token == nil {
		return ""
	}
	return strconvFormatInt(*token)
}

func dashboardQueryFromRequest(r *http.Request, limit int) (dashboardQuery, error) {
	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to"))

	var query dashboardQuery
	if limit > 0 {
		query.Limit = limit
	}

	if fromRaw != "" {
		from, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			return dashboardQuery{}, errors.New("invalid from timestamp (must be RFC3339)")
		}
		query.From = &from
	}
	if toRaw != "" {
		to, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			return dashboardQuery{}, errors.New("invalid to timestamp (must be RFC3339)")
		}
		query.To = &to
	}

	if query.From != nil && query.To != nil && !query.From.Before(*query.To) {
		return dashboardQuery{}, errors.New("invalid range: from must be earlier than to")
	}

	return query, nil
}

type configuredUser struct {
	ID       string
	KeyAlias string
}

func (s *Server) configuredUsers() []configuredUser {
	records, err := s.management.listTenantKeys()
	if err != nil {
		s.logger.Error("failed to list managed user keys for dashboard overview", "error", err)
		return nil
	}

	userByID := make(map[string]configuredUser, len(records))
	for _, record := range records {
		if _, exists := userByID[record.UserID]; exists {
			continue
		}
		userByID[record.UserID] = configuredUser{ID: record.UserID, KeyAlias: record.SecretMask}
	}

	users := make([]configuredUser, 0, len(userByID))
	for _, user := range userByID {
		users = append(users, user)
	}

	sort.Slice(users, func(i, j int) bool {
		if users[i].ID == users[j].ID {
			return users[i].KeyAlias < users[j].KeyAlias
		}
		return users[i].ID < users[j].ID
	})
	return users
}

func dashboardRangeResponse(query dashboardQuery) map[string]any {
	response := map[string]any{}
	if query.From != nil {
		response["from"] = query.From.UTC().Format(time.RFC3339)
	}
	if query.To != nil {
		response["to"] = query.To.UTC().Format(time.RFC3339)
	}
	return response
}

func maskTenantKey(key string) string {
	if len(key) <= 3 {
		return "***"
	}
	if len(key) <= 8 {
		return key[:1] + "***" + key[len(key)-1:]
	}
	return key[:4] + "***" + key[len(key)-2:]
}

func prettyJSON(raw []byte) string {
	var compacted bytes.Buffer
	if err := json.Indent(&compacted, raw, "", "  "); err != nil {
		return string(raw)
	}
	return compacted.String()
}

func strconvFormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}
