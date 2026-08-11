package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"truemoney-voucher/internal/config"
	"truemoney-voucher/internal/server"
	"truemoney-voucher/internal/truemoney"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tm := truemoney.NewClient()
	// Keep a pooled connection + cf_clearance warm so redeems after an
	// idle gap do not pay the ~120 ms connection setup cost (see
	// truemoney.StartWarmer). Interval matches the Go-Product compose
	// default (15 s), well under the httpx 30 s idle timeout.
	go tm.StartWarmer(ctx, 15*time.Second, logger)
	srv := server.New(cfg, logger, tm)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		logger.Error("server error", "err", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	logger.Info("server stopped")
}