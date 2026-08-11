package server

import (
	"errors"
	"log/slog"
	"net/http"

	"truemoney-voucher/internal/config"
	"truemoney-voucher/internal/middleware"
	"truemoney-voucher/internal/model"
	"truemoney-voucher/internal/truemoney"
	"truemoney-voucher/pkg/response"
)

// api carries the dependencies shared by all handlers.
type api struct {
	cfg    *config.Config
	logger *slog.Logger
	tm     *truemoney.Client
}

// NewRouter registers all routes and wraps them in the middleware chain.
// Route method-patterns ("GET /truemoney/{code}/{mobile}") require Go 1.22+.
func NewRouter(cfg *config.Config, logger *slog.Logger, tm *truemoney.Client) http.Handler {
	h := &api{cfg: cfg, logger: logger, tm: tm}

	mux := http.NewServeMux()

	// Voucher redemption: GET and POST are equivalent — both redeem.
	// {code} accepts a raw gift code or a full campaign URL
	// (URL-encoded in the path); {mobile} is a Thai mobile number.
	mux.HandleFunc("GET /truemoney/{code}/{mobile}", h.handleRedeem)
	mux.HandleFunc("POST /truemoney/{code}/{mobile}", h.handleRedeem)

	// Liveness probe for load balancers / uptime monitors.
	mux.HandleFunc("GET /status", h.handleStatus)
	mux.HandleFunc("POST /status", h.handleStatus)

	// Root endpoint: basic service information. {$} matches exactly "/",
	// so unknown paths still fall through to the JSON 404 below.
	mux.HandleFunc("GET /{$}", h.handleRoot)
	mux.HandleFunc("POST /{$}", h.handleRoot)

	// Everything else is a JSON 404 (API-only service).
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeAppError(w, model.ErrNotFound)
	})

	return middleware.CORS(middleware.Recover(logger)(middleware.Logging(logger)(mux)))
}

// handleRedeem redeems a voucher to the given Thai mobile number.
// PathValue decodes URL-escaped segments, so {code} also accepts a full
// gift.truemoney.com link like
// https%3A%2F%2Fgift.truemoney.com%2Fcampaign%2F%3Fv%3D<code>.
func (h *api) handleRedeem(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	mobile := r.PathValue("mobile")
	if code == "" || mobile == "" {
		writeAppError(w, model.ErrBadRequest)
		return
	}

	result, err := h.tm.Redeem(r.Context(), code, mobile)
	if err != nil {
		h.logger.Error("redeem failed", "err", err, "code", maskCode(code))
		// Input validation failures (bad code/mobile) are client errors (400);
		// everything else (upstream, I/O, encoding) is a server error (500).
		var verr *truemoney.ValidationError
		if errors.As(err, &verr) {
			writeAppError(w, model.ErrBadRequest)
		} else {
			writeAppError(w, model.ErrInternal)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(result)
}

// handleStatus is the liveness probe.
func (h *api) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleRoot answers with basic service information.
func (h *api) handleRoot(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]any{
		"service": "truemoney-voucher",
		"routes": []string{
			"GET|POST /truemoney/{code}/{mobile}  redeem voucher",
			"GET|POST /status                     liveness probe",
		},
	})
}

// maskCode hides all but the first and last four characters of a voucher
// code, since a voucher code is cash-equivalent and must not appear in
// plaintext in logs.
func maskCode(code string) string {
	if len(code) <= 8 {
		return "****"
	}
	return code[:4] + "****" + code[len(code)-4:]
}

// writeAppError writes an AppError as JSON. Client-facing HTTP and body
// status codes follow the upstream proxy convention.
func writeAppError(w http.ResponseWriter, err *model.AppError) {
	response.JSON(w, err.Status, err)
}