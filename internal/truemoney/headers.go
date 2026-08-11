package truemoney

import "net/http"

const campaignPrefix = "https://gift.truemoney.com/campaign/?v="

// setBrowserHeaders fills the request with Firefox-compatible headers.
// The UA version must stay in sync with the TLS fingerprint and HTTP/2
// SETTINGS in internal/httpx (Firefox 148 in all three places).
// Note: Firefox does not send the Chrome-only "Priority" header.
func setBrowserHeaders(req *http.Request, accept string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:148.0) Gecko/20100101 Firefox/148.0")
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}