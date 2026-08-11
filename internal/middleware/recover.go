package middleware

import (
	"log/slog"
	"net/http"

	"truemoney-voucher/pkg/response"
)

// recoveryWriter tracks whether a response has already started, so the
// panic handler never writes a 500 after headers were flushed.
type recoveryWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *recoveryWriter) WriteHeader(status int) {
	if !w.wrote {
		w.wrote = true
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *recoveryWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Recover catches panics so a single bad request cannot crash the process.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recoveryWriter{ResponseWriter: w}
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"err", err,
						"path", r.URL.Path,
						"method", r.Method,
					)
					if !rw.wrote {
						response.JSON(w, http.StatusInternalServerError, map[string]any{
							"code":    500,
							"message": "Internal Server Error",
						})
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}