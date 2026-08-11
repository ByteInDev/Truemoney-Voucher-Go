<br>

<div align="center">

# Truemoney-Voucher

**REST API for redeeming TrueMoney gift vouchers** — Go, no database, stdlib only

![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)
![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)

**English** - [Thai](README.th.md)

</div>

---

A minimal Go API that talks to `gift.truemoney.com` through a transport that
mimics a real Firefox 148 browser, at both the TLS and HTTP/2 wire level, so
requests pass Cloudflare bot detection. One operation only: **redeem** a
voucher to a Thai mobile number.

## Features

| Ability | Details |
| ------- | ------- |
| Redeem | `GET`/`POST /truemoney/{code}/{mobile}` - redeem to a mobile number (both methods are equivalent) |
| Raw code or full link | accepts `gift.truemoney.com/campaign/?v=<code>` URLs too |
| Input validation | code <= 128 chars; Thai mobile: 10 digits starting with `0` |
| Cloudflare bypass | uTLS `HelloFirefox_148` fingerprint + hand-built HTTP/2 framer |
| Safe by design | codes masked in logs, gzip/deflate/br handled, graceful shutdown |

## Quick Start

```bash
go run ./cmd/api                     # listens on :3000
```

```bash
docker build -t truemoney-voucher -f deployments/Dockerfile .
docker run -d -p 3000:3000 truemoney-voucher
```

Check it is alive:

```bash
curl localhost:3000/status           # 200 OK (empty)
curl localhost:3000/                 # service info + routes
```

## API Reference

### Endpoints

| Method | Path | Description |
| ------ | ---- | ----------- |
| `GET` / `POST` | `/truemoney/{code}/{mobile}` | Redeem a voucher |
| `GET` / `POST` | `/status` | Liveness probe |
| `GET` / `POST` | `/` | Service info and route list |

### Path parameters

| Param | Accepted format |
| ----- | --------------- |
| `code` | Raw code (alnum + `-`/`_`, <= 128 chars) or URL-encoded full link `https://gift.truemoney.com/campaign/?v=<code>` |
| `mobile` | Thai mobile: 10 digits starting with `0` (spaces/dashes auto-stripped) |

### Examples

```bash
# Redeem with a raw code - GET and POST are equivalent
curl "localhost:3000/truemoney/ABCD1234EFGH/0812345678"
curl -X POST "localhost:3000/truemoney/ABCD1234EFGH/0812345678"

# Redeem with a URL-encoded full link
curl "localhost:3000/truemoney/https%3A%2F%2Fgift.truemoney.com%2Fcampaign%2F%3Fv%3DABCD1234EFGH/0812345678"
```

### Responses

TrueMoney's JSON is passed through unchanged (including its `{"status": {...}}`
error envelope). Own errors are always `code` + `message`:

| HTTP status | Body | When |
| ----------- | ---- | ---- |
| `200` | `{"code": 400, "message": "Bad Request"}` | invalid code / mobile |
| `404` | `{"code": 404, "message": "Not Found"}` | unknown path |
| `200` | `{"code": 500, "message": "Internal Server Error"}` | TrueMoney call failed |
| `500` | `{"code": 500, "message": "Internal Server Error"}` | panic recovered |

### TrueMoney status codes

Inside `status.code` of the envelope:

| Code | Meaning |
| ---- | ------- |
| `SUCCESS` | Money received successfully |
| `TARGET_USER_REDEEMED` | You already redeemed this voucher |
| `VOUCHER_OUT_OF_STOCK` | Someone else already took it |
| `VOUCHER_EXPIRED` | The wallet voucher has expired |
| `VOUCHER_NOT_FOUND` | Voucher not found in the system |
| `CANNOT_GET_OWN_VOUCHER` | Cannot redeem your own voucher |
| `TARGET_USER_NOT_FOUND` | Phone number not found in the system |
| `INTERNAL_ERROR` | Voucher not found, or the URL is wrong |

## Configuration

| Env var | Default | Description |
| ------- | ------- | ----------- |
| `PORT` | `3000` | HTTP listen port (1-65535) |

```bash
PORT=8080 go run ./cmd/api
```

## Deploy to Vercel

The Vercel [Go Framework Preset](https://vercel.com/docs/functions/runtimes/go)
runs the server as-is — it picks the `go` version from `go.mod` (1.26),
builds `cmd/api/main.go`, and the server listens on the `PORT` Vercel
provides. No code changes:

```bash
vercel --prod
```

`vercel.json` only sets `"framework": "go"`.

**Serverless caveats** — each invocation is a fresh process, so:

- the TLS/HTTP2 connection pool and the shared `cf_clearance` cookie jar
  start cold on every request; Cloudflare behaviour and latency may differ
  from Docker/VPS
- `%2F`-encoded redeem links may be normalized by the platform before they
  reach the server — prefer `curl --path-as-is` (works on Docker/VPS)

**Performance on the Free (Hobby) plan:** the binary is stripped with
`GO_BUILD_FLAGS=-ldflags '-s -w'` (faster cold start) and functions run in
`iad1` (US East) only — Thailand→Virginia RTT (~200 ms) is fixed and
unavoidable on a free plan. Measure with a keep-alive client, not a fresh
`curl.exe` per request, to see actual server time.

## Build and Deploy

```bash
make run           # go run ./cmd/api
make build         # CGO_ENABLED=0 go build -o bin/api ./cmd/api
make vet           # go vet ./...
make docker-build  # docker build -t truemoney-voucher
make deploy-local  # docker run -d -p 3000:3000 truemoney-voucher
make deploy        # cross-compile + ssh/scp to a remote server
                   # (host/user hardcoded in the Makefile - edit first!)
make vercel-deploy # vercel --prod (serverless)
```

## Architecture (tl;dr)

- **`internal/truemoney`** — TrueMoney domain logic: endpoints, validation,
  response handling. One shared client + cookie jar (keeps `cf_clearance` warm).
- **`internal/httpx`** — custom `http.RoundTripper` speaking HTTP/2 like
  Firefox 148: uTLS `HelloFirefox_148` (defeats JA3/JA4), Firefox SETTINGS and
  fixed header order (manual HPACK), idle pool (4 conns/host), TLS session
  resumption, one retry on stale connections.
- **`internal/server`** — `net/http` (Go 1.22+ method patterns), timeouts
  15s/10s/30s/60s, middleware `CORS -> Recover -> Logging`, graceful shutdown (10s).

## Testing

```bash
make vet           # go vet ./... (static analysis)
```

Unit tests are not implemented yet.

## Disclaimer

> **For educational use or where the provider permits it.**
> Redeeming is irreversible and governed by TrueMoney's Terms of Service.
> Voucher codes are cash-equivalent — never expose logs containing full codes.

## Contributing

Contributions are welcome! Please:

1. Open an issue first for significant changes
2. Keep `go vet ./...` green
3. Follow the existing code style

## License

Licensed under the [MIT License](./LICENSE) © 2026 ByteInDev