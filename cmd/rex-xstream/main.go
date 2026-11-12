package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leleasley/rex-xstream/internal/api"
	"github.com/leleasley/rex-xstream/internal/config"
	"github.com/leleasley/rex-xstream/internal/database"
	"github.com/leleasley/rex-xstream/internal/probe"
	"github.com/leleasley/rex-xstream/internal/rex"
	"github.com/leleasley/rex-xstream/internal/security"
	"github.com/leleasley/rex-xstream/internal/store"
	xsync "github.com/leleasley/rex-xstream/internal/sync"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Version == "dev" {
		cfg.Version = version
	}
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	cipher, err := security.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	st := store.New(db, cipher)
	if err := st.MarkInterruptedRuns(ctx); err != nil {
		return err
	}
	rc := rex.New(cfg.RexURL, cfg.RexSourceName, cfg.RexSourceUser, cfg.RexSourcePassword, cfg.RexPushBatch)
	rc.SetToken(cfg.RexToken)
	syncer := xsync.New(st, cfg, rc, probe.New(cfg.FFprobePath, cfg.ProbeTimeout), logger)
	if cfg.SyncOnStart {
		syncer.StartAll(ctx)
	}
	if cfg.SyncInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.SyncInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					syncer.StartAll(ctx)
				}
			}
		}()
	}
	server := &http.Server{Addr: cfg.Addr(), Handler: api.New(cfg, st, syncer, logger).Handler(), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	logger.Info("server started", "address", cfg.Addr(), "version", cfg.Version)
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
	return nil
}
