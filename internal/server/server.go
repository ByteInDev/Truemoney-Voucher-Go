package server

import (
	"log/slog"
	"net/http"
	"time"

	"truemoney-voucher/internal/config"
	"truemoney-voucher/internal/truemoney"
)

// New builds the http.Server with sane timeouts and the configured router.
func New(cfg *config.Config, logger *slog.Logger, tm *truemoney.Client) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewRouter(cfg, logger, tm),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}