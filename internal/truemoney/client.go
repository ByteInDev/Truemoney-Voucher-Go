// Package truemoney is the TrueMoney gift voucher API client.
//
// It calls https://gift.truemoney.com (protected by Cloudflare) through
// internal/httpx, a transport that mimics a real browser at the TLS and
// HTTP/2 wire level. This package only contains TrueMoney domain logic:
// endpoints, payloads, headers and response handling.
package truemoney

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"github.com/andybalholm/brotli"

	"truemoney-voucher/internal/httpx"
)

// gzipReaderPool reuses gzip readers so decompressing upstream responses
// does not allocate the reader state per request.
var gzipReaderPool = sync.Pool{
	New: func() any {
		r, _ := gzip.NewReader(bytes.NewReader(nil))
		return r
	},
}

func getGzipReader(r io.Reader) (*gzip.Reader, error) {
	if gr, ok := gzipReaderPool.Get().(*gzip.Reader); ok {
		if err := gr.Reset(r); err != nil {
			return nil, err
		}
		return gr, nil
	}
	return gzip.NewReader(r)
}

func putGzipReader(gr *gzip.Reader) {
	_ = gr.Close()
	gzipReaderPool.Put(gr)
}

// Client performs TrueMoney voucher API calls over a browser-mimicking
// transport. It is safe for concurrent use.
//
// Note: one Client is shared by the whole service, so its cookie jar is
// common across all users. That is intentional — a warm cf_clearance
// improves stability against Cloudflare — but cookies are not isolated
// per caller.
type Client struct {
	http  *http.Client
	cache *redeemCache
	// probeURL overrides the upstream base URL in tests so probes never
	// touch the real endpoint; empty means https://gift.truemoney.com.
	probeURL string
}

// NewClient builds a Client with the browser fingerprint transport.
// Successful redeem answers are cached for ten minutes (see cache.go).
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
			Transport: httpx.NewTransport(),
		},
		cache: newRedeemCache(10*time.Minute, 1024),
	}
}

// doJSON performs the request and returns the raw JSON response body.
//
// gzip/deflate/brotli are decoded manually because the custom HTTP/2
// transport does not auto-decompress. An empty 2xx body (which TrueMoney
// sometimes returns, e.g. for already-redeemed vouchers) becomes {}.
func (c *Client) doJSON(req *http.Request) (json.RawMessage, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gr, gzErr := getGzipReader(resp.Body)
		if gzErr != nil {
			return nil, fmt.Errorf("gzip reader: %w", gzErr)
		}
		defer putGzipReader(gr)
		data, err := io.ReadAll(io.LimitReader(gr, 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return validJSON(data, resp.StatusCode)
	case "deflate":
		data, err := io.ReadAll(io.LimitReader(flate.NewReader(resp.Body), 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return validJSON(data, resp.StatusCode)
	case "br":
		data, err := io.ReadAll(io.LimitReader(brotli.NewReader(resp.Body), 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return validJSON(data, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return validJSON(data, resp.StatusCode)
}

// validJSON validates and returns the response body, including a preview
// of unexpected (non-JSON) responses such as Cloudflare challenges.
//
// TrueMoney itself answers domain errors (e.g. TARGET_USER_NOT_FOUND) with
// HTTP 400 + a JSON "status" envelope, so non-2xx bodies carrying that
// envelope still pass through. Anything else on a >=400 status — Cloudflare
// challenges or upstream errors without the envelope — becomes an error
// whose text embeds the upstream HTTP status.
func validJSON(data []byte, statusCode int) (json.RawMessage, error) {
	if len(data) == 0 {
		if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
			return json.RawMessage(`{}`), nil
		}
		return nil, fmt.Errorf("TrueMoney returned HTTP %d with an empty body", statusCode)
	}

	if !json.Valid(data) {
		preview := string(data)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("TrueMoney returned HTTP %d with a non-JSON response: %s", statusCode, preview)
	}

	if statusCode >= http.StatusBadRequest && !isTrueMoneyEnvelope(data) {
		preview := string(data)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("upstream returned HTTP %d without a TrueMoney status envelope: %s", statusCode, preview)
	}

	return json.RawMessage(data), nil
}

// envelopeStatusNeedle is the JSON key that identifies a TrueMoney-style
// status envelope; hoisted so bytes.Contains does not allocate a needle
// per check.
var envelopeStatusNeedle = []byte(`"status"`)

// isTrueMoneyEnvelope reports whether data is TrueMoney-style JSON, i.e.
// it carries a "status" object (TrueMoney's own error convention).
func isTrueMoneyEnvelope(data []byte) bool {
	return bytes.Contains(data, envelopeStatusNeedle)
}