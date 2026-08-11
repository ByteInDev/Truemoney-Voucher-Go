package httpx

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/http2/hpack"
)

// encodeHeaders resets the scratch buffer and HPACK-encodes the request
// headers in browser order into it, returning a copy of the block. The
// encoder is persistent so its dynamic table stays in sync with the
// server's decoder for the life of the connection (RFC 9113 §4.3).
func (c *h2conn) encodeHeaders(req *http.Request, headerOrder []string) ([]byte, error) {
	c.hbuf.Reset()
	if err := encodeHeaderBlock(c.enc, req, headerOrder); err != nil {
		return nil, err
	}
	// Copy out: hbuf is reused by the next encode.
	return append([]byte(nil), c.hbuf.Bytes()...), nil
}

// encodeHeaderBlock HPACK-encodes request headers into the encoder's
// writer. Headers are encoded manually because Go's http.Header iterates
// randomly; headerOrder fixes the wire order.
func encodeHeaderBlock(enc *hpack.Encoder, req *http.Request, headerOrder []string) error {
	// Pseudo-headers in required order: :method, :scheme, :authority, :path.
	for _, f := range [][2]string{
		{":method", req.Method},
		{":scheme", "https"},
		{":authority", req.URL.Host},
		{":path", req.URL.RequestURI()},
	} {
		if err := enc.WriteField(hpack.HeaderField{Name: f[0], Value: f[1]}); err != nil {
			return fmt.Errorf("hpack encode %s: %w", f[0], err)
		}
	}

	// Regular headers in browser order. Some headers are never put on the
	// wire in HTTP/2; they are handled via pseudo-headers or framing.
	skip := map[string]bool{
		"Content-Length": true,
		"Connection":     true,
		"Host":           true,
	}
	for _, key := range headerOrder {
		canonical := http.CanonicalHeaderKey(key)
		if skip[canonical] {
			continue
		}
		values := req.Header[canonical]
		if len(values) == 0 {
			if canonical == "Accept-Encoding" {
				values = []string{"gzip, deflate, br"}
			} else {
				continue
			}
		}
		skip[canonical] = true
		for _, v := range values {
			if err := enc.WriteField(hpack.HeaderField{Name: strings.ToLower(key), Value: v}); err != nil {
				return fmt.Errorf("hpack encode %s: %w", key, err)
			}
		}
	}

	return nil
}

// decodeResponse decodes a HEADERS block into an *http.Response using the
// persistent decoder. The decoder's dynamic table survives across blocks
// and its limit (65536) matches the HEADER_TABLE_SIZE we advertise in
// SETTINGS, so the server may use the full advertised table.
func (c *h2conn) decodeResponse(block []byte, req *http.Request, body io.Reader) (*http.Response, error) {
	if len(block) == 0 {
		return nil, fmt.Errorf("empty response: no HEADERS frame received")
	}
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		Header:     make(http.Header),
		Body:       io.NopCloser(body),
		Request:    req,
	}

	c.dec.SetEmitFunc(func(f hpack.HeaderField) {
		switch {
		case f.Name == ":status":
			resp.StatusCode = parseStatus(f.Value)
			resp.Status = f.Value + " " + http.StatusText(resp.StatusCode)
		case strings.HasPrefix(f.Name, ":"):
			// Skip other pseudo-headers.
		default:
			resp.Header.Add(f.Name, f.Value)
		}
	})

	if _, err := c.dec.Write(block); err != nil {
		return nil, fmt.Errorf("hpack decode: %w", err)
	}

	// :status is mandatory; parseStatus yields 0 for anything non-numeric
	// (e.g. a malformed value), which is a protocol violation.
	if resp.StatusCode == 0 {
		return nil, fmt.Errorf("response missing or invalid :status pseudo-header")
	}

	return resp, nil
}

func parseStatus(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}