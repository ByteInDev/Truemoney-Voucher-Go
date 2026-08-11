package truemoney

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// StartWarmer keeps at least one pooled connection and a warm cf_clearance
// alive so real redeems almost always hit a warm connection (parity with
// the Go-Product version's warmer).
//
// Without it, the first redeem after an idle gap pays the full connection
// setup cost (dial + TLS handshake + HTTP/2, roughly 120 ms). The warmer
// issues one probe (a deliberately invalid redeem) every interval; each
// call refreshes the pooled connection and the shared cookie jar, and
// never pollutes the redeem cache (probe answers are error envelopes).
//
// It runs until ctx is cancelled; failures are logged but do not stop the
// loop. A nil logger disables the failure log line.
func (c *Client) StartWarmer(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastFailure := time.Time{}
	const failureWindow = 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := c.probeOnce(probeCtx)
			cancel()
			if err == nil {
				lastFailure = time.Time{}
				continue
			}
			if logger != nil && time.Since(lastFailure) > failureWindow {
				logger.Warn("connection warmer probe failed", "err", err)
				lastFailure = time.Now()
			}
		}
	}
}

// probeOnce sends a redeem for a fixed, deliberately invalid voucher:
// TrueMoney answers with a JSON error envelope, which proves the request
// passed Cloudflare and received a real upstream response.
func (c *Client) probeOnce(ctx context.Context) error {
	const (
		probeCode   = "PROBE000000"
		probeMobile = "0000000000"
	)

	base := c.probeURL
	if base == "" {
		base = "https://gift.truemoney.com"
	}
	url := fmt.Sprintf("%s/campaign/vouchers/%s/redeem", base, probeCode)
	body, err := json.Marshal(map[string]string{"mobile": probeMobile})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	setBrowserHeaders(req, "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://gift.truemoney.com/campaign/card")

	_, err = c.doJSON(req)
	return err
}