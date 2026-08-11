package httpx

import (
	utls "github.com/refraction-networking/utls"
)

// Browser TLS and HTTP/2 fingerprint settings, tuned to match a real
// Firefox 148 (current release family in 2026): TLS ClientHello, SETTINGS,
// header order and the User-Agent (set in internal/truemoney) must all
// move together — a mixed fingerprint is detectable.
var (
	browserFingerprint = utls.HelloFirefox_148

	firefoxHeaderOrder = []string{
		"User-Agent",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
		"Content-Type",
		"Referer",
		"Sec-Fetch-Dest",
		"Sec-Fetch-Mode",
		"Sec-Fetch-Site",
	}
)

// NOTE on spec reuse: never cache a ClientHelloSpec and share it across
// connections. uTLS's HelloCustom mode writes GREASE/random values into the
// spec structs in place, so a reused spec breaks every handshake after the
// first. UClient with a ClientHelloID generates a fresh spec per connection,
// which keeps ALPN h2 while staying safe for concurrent use.