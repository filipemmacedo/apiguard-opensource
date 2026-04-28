package main

import (
	"apiguard/internal/config"
	"apiguard/internal/proxy"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	if err := loadEnvFile(".env"); err != nil {
		slog.Error("failed to load .env file", "error", err)
		os.Exit(1)
	}

	cfg, err := config.LoadFromEnv(os.Getenv)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := proxy.NewServer(cfg, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if err := server.Close(); err != nil {
			logger.Error("failed to close persistent storage", "error", err)
		}
	}()
	if cfg.EnableTestUI {
		logger.Warn("internal test interface enabled", "path", "/internal/test-interface")
	}

	server.Start(ctx)

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: server.Handler(),
	}

	logger.Info("api guard starting", "listen_addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}
