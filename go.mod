module truemoney-voucher

go 1.26

// uTLS is pinned to a pseudo-version (master) because HelloFirefox_148 —
// the fingerprint this project uses — exists only on master as of the
// pinned date; the newest tagged release (v1.8.2) predates it.
require (
	github.com/andybalholm/brotli v1.2.2
	github.com/refraction-networking/utls v1.8.3-0.20260802151714-23b1dac19c06
	golang.org/x/net v0.57.0
)

require (
	github.com/klauspost/compress v1.17.11 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
