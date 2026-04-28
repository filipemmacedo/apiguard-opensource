package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"apiguard/internal/config"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testDatabaseDSN returns the PostgreSQL DSN for tests.
// Tests that require a database will be skipped if DATABASE_DSN is not set.
func testDatabaseDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("DATABASE_DSN not set — skipping test that requires a PostgreSQL database")
	}
	return dsn
}

func newTestServer(t *testing.T, cfg config.Config, logger *slog.Logger) *Server {
	t.Helper()

	if cfg.DatabaseDSN == "" {
		cfg.DatabaseDSN = testDatabaseDSN(t)
	}

	server := NewServer(cfg, logger)
	t.Cleanup(func() {
		_ = server.Close()
	})
	return server
}

func newOpenAIManagedProviderTestServer(t *testing.T, chatHandler http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini","owned_by":"openai"},{"id":"gpt-4.1","owned_by":"openai"}]}`))
		case "/v1/chat/completions":
			chatHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

func withManagedProviderBootstrap(cfg config.Config, upstreamURL string) config.Config {
	if cfg.UpstreamTimeout <= 0 {
		cfg.UpstreamTimeout = time.Second
	}
	if cfg.UpstreamAPIKey == "" {
		cfg.UpstreamAPIKey = "upstream-key"
	}
	if cfg.SecretMasterKey == "" {
		cfg.SecretMasterKey = "test-master-secret"
	}
	if cfg.OpenAIBaseURL == "" {
		cfg.OpenAIBaseURL = upstreamURL
	}
	cfg.UpstreamBaseURL = upstreamURL
	return cfg
}
