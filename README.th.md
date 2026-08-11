<br>

<div align="center">

# Truemoney-Voucher (Go)

**REST API สำหรับแลกรับ TrueMoney Gift Voucher** — Go, ไม่มี database, ใช้แค่ stdlib

![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)
![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
[![Live on Vercel](https://img.shields.io/badge/Live-Vercel-000000?logo=vercel&logoColor=white)](https://truemoney-voucher-go.vercel.app)

[English](README.md) - **ไทย**

</div>

---

บริการ API ขนาดเล็กที่เรียก `gift.truemoney.com` ผ่าน transport ที่จำลองเบราว์เซอร์
Firefox 148 ของจริง — ทั้งในระดับ TLS และ HTTP/2 wire level — เพื่อให้คำขอผ่าน
Cloudflare bot detection ได้ มีเพียงคำสั่งเดียว: **แลกรับโค้ด (redeem)**
เข้ากับเบอร์โทรศัพท์ไทย

## ความสามารถ

| ความสามารถ | รายละเอียด |
| ----------- | ----------- |
| แลกรับโค้ด | `GET`/`POST /truemoney/{code}/{mobile}` — แลกเข้ากับเบอร์โทรศัพท์ (GET กับ POST ให้ผลเหมือนกัน) |
| รองรับลิงก์เต็ม | ใส่ลิงก์ `gift.truemoney.com/campaign/?v=<code>` ได้ด้วย |
| ตรวจสอบ input | โค้ด ≤ 128 ตัวอักษร; เบอร์ไทย 10 หลักขึ้นต้นด้วย `0` |
| ผ่าน Cloudflare | fingerprint uTLS `HelloFirefox_148` + HTTP/2 framer ที่เขียนเอง |
| ปลอดภัย | โค้ดถูก mask ใน log, รองรับ gzip/deflate/br, graceful shutdown |

## เริ่มต้นใช้งาน

```bash
go run ./cmd/api                     # ฟังที่ :3000
```

```bash
docker build -t truemoney-voucher -f deployments/Dockerfile .
docker run -d -p 3000:3000 truemoney-voucher
```

ทดสอบว่า service ทำงาน:

```bash
curl localhost:3000/status           # 200 OK (ว่างเปล่า)
curl localhost:3000/                 # ข้อมูลบริการ + รายการ routes
```

## API Reference

### Endpoints

| Method | Path | คำอธิบาย |
| ------ | ---- | -------- |
| `GET` / `POST` | `/truemoney/{code}/{mobile}` | แลกรับโค้ด (redeem) |
| `GET` / `POST` | `/status` | Liveness probe |
| `GET` / `POST` | `/` | ข้อมูลบริการและรายการ routes |

### พารามิเตอร์ใน path

| พารามิเตอร์ | รูปแบบที่รับได้ |
| ----------- | --------------- |
| `code` | raw code (ตัวอักษร/ตัวเลข + `-`/`_` ยาว ≤ 128 ตัว) หรือลิงก์เต็ม `https://gift.truemoney.com/campaign/?v=<code>` ที่ URL-encode แล้ว |
| `mobile` | เบอร์ไทย 10 หลักขึ้นต้นด้วย `0` (เว้นวรรค/ขีดคั่นถูกลบให้อัตโนมัติ) |

### ตัวอย่าง

```bash
# แลกรับด้วย raw code — GET หรือ POST ก็ได้ ผลเหมือนกัน
curl "localhost:3000/truemoney/ABCD1234EFGH/0812345678"
curl -X POST "localhost:3000/truemoney/ABCD1234EFGH/0812345678"

# แลกรับด้วยลิงก์เต็มที่ URL-encode แล้ว
curl "localhost:3000/truemoney/https%3A%2F%2Fgift.truemoney.com%2Fcampaign%2F%3Fv%3DABCD1234EFGH/0812345678"
```

### รูปแบบ response

JSON ที่ TrueMoney ตอบกลับถูกส่งผ่าน (passthrough) ตามเดิม รวมถึง error envelope
`{"status": {...}}` ส่วน error ของตัว API เองตอบเป็น `code` + `message` เสมอ:

| HTTP status | Body | เมื่อใด |
| ----------- | ---- | ------- |
| `200` | `{"code": 400, "message": "Bad Request"}` | โค้ด/เบอร์ไม่ถูกต้อง |
| `404` | `{"code": 404, "message": "Not Found"}` | path ไม่รู้จัก |
| `200` | `{"code": 500, "message": "Internal Server Error"}` | เรียก TrueMoney แล้วพลาด |
| `500` | `{"code": 500, "message": "Internal Server Error"}` | panic ถูก recover |

### รหัสสถานะจาก TrueMoney

อยู่ใน `status.code` ของ envelope:

| รหัสสถานะ | ความหมาย |
| ---------- | -------- |
| `SUCCESS` | รับเงินสำเร็จ |
| `TARGET_USER_REDEEMED` | คุณรับซองนี้ไปแล้ว |
| `VOUCHER_OUT_OF_STOCK` | มีคนรับไปแล้ว |
| `VOUCHER_EXPIRED` | ซองวอเลทหมดอายุแล้ว |
| `VOUCHER_NOT_FOUND` | ไม่พบซองในระบบ |
| `CANNOT_GET_OWN_VOUCHER` | รับซองตัวเองไม่ได้ |
| `TARGET_USER_NOT_FOUND` | ไม่พบเบอร์ในระบบ |
| `INTERNAL_ERROR` | ไม่พบซองในระบบ หรือ URL ผิด |

## การตั้งค่า

| ตัวแปร env | ค่าเริ่มต้น | รายละเอียด |
| ----------- | ----------- | ----------- |
| `PORT` | `3000` | พอร์ตที่ HTTP server ฟัง (1-65535) |

```bash
PORT=8080 go run ./cmd/api
```

## Build และ Deploy

```bash
make run           # go run ./cmd/api
make build         # CGO_ENABLED=0 go build -o bin/api ./cmd/api
make vet           # go vet ./...
make docker-build  # docker build -t truemoney-voucher
make deploy-local  # docker run -d -p 3000:3000 truemoney-voucher
make deploy        # cross-compile + ssh/scp ไปยัง remote server
                   # (host/user ฝังใน Makefile - แก้ไขก่อนใช้งาน!)
make vercel-deploy # vercel --prod (serverless)
```

## Deploy บน Vercel

Vercel [Go Framework Preset](https://vercel.com/docs/functions/runtimes/go)
รันเซิร์ฟเวอร์ตามเดิม — เลือกเวอร์ชัน `go` จาก `go.mod` (1.26) สร้าง
`cmd/api/main.go` และเซิร์ฟเวอร์ฟังพอร์ต `PORT` ที่ Vercel จัดให้
ไม่ต้องแก้โค้ดเลย:

```bash
vercel --prod
```

`vercel.json` ตั้งแค่ `"framework": "go"` เท่านั้น

**ข้อควรระวังแบบ serverless** — แต่ละ invocation เป็น process ใหม่ ดังนั้น:

- TLS/HTTP2 connection pool และ shared cookie jar `cf_clearance` เริ่มเย็น
  ทุก request; พฤติกรรมกับ Cloudflare และ latency อาจต่างจาก Docker/VPS
- ลิงก์ redeem ที่ encode `%2F` อาจถูก platform normalize ก่อนถึงเซิร์ฟเวอร์ —
  แนะนำ `curl --path-as-is` (บน Docker/VPS ทำงานปกติ)

**ประสิทธิภาพบน Free (Hobby) plan:** binary ถูก strip ด้วย
`GO_BUILD_FLAGS=-ldflags '-s -w'` (cold start เร็วขึ้น) และฟังก์ชันรันได้แค่
`iad1` (US East) — RTT ไทย→เวอร์จิเนีย (~200 ms) แก้ไม่ได้บน free plan
วัดด้วย client แบบ keep-alive อย่าใช้ `curl.exe` ใหม่ทุกครั้งเพื่อดูเวลา
server จริง

## สถาปัตยกรรม (โดยย่อ)

- **`internal/truemoney`** — ตรรกะฝั่ง TrueMoney: endpoints, validation,
  การจัดการ response ใช้ client + cookie jar ตัวเดียวร่วมกัน (ทำให้ `cf_clearance`
  อุ่นอยู่เสมอ)
- **`internal/httpx`** — `http.RoundTripper` ที่พูด HTTP/2 แบบ Firefox 148:
  fingerprint uTLS `HelloFirefox_148` (รอด JA3/JA4), SETTINGS + ลำดับ header
  แบบ Firefox (HPACK เขียนเอง), idle pool (4 conns/host), TLS session resumption,
  retry 1 ครั้งเมื่อ connection เก่าใช้งานไม่ได้
- **`internal/server`** — `net/http` (method pattern Go 1.22+), timeouts
  15s/10s/30s/60s, middleware `CORS -> Recover -> Logging`, graceful shutdown (10s)

## การทดสอบ

```bash
make vet           # go vet ./... (ตรวจสอบ static analysis)
```

ยังไม่มี unit tests ในตอนนี้

## ข้อควรระวัง

> **ใช้เพื่อการศึกษาหรือในกรณีที่ผู้ให้บริการอนุญาตเท่านั้น**
> การแลกรับโค้ดไม่สามารถย้อนกลับได้ และอยู่ภายใต้ข้อกำหนดการใช้งาน (ToS) ของ TrueMoney
> โค้ดของขวัญมีค่าเทียบเท่าเงินสด — อย่าเปิดเผย log ที่มีโค้ดเต็มสู่สาธารณะ

## การมีส่วนร่วม

ยินดีต้อนรับทุกการมีส่วนร่วมครับ:

1. กรุณาเปิด issue เพื่อหารือก่อนสำหรับการเปลี่ยนแปลงที่มีนัยสำคัญ
2. รักษา `go vet ./...` ให้ผ่านเสมอ
3. ปฏิบัติตาม code style ที่มีอยู่

## สิทธิ์การใช้งาน

ใช้ภายใต้สัญญาอนุญาต [MIT License](./LICENSE) © 2026 ByteInDev