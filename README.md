# lignin

[![Go Reference](https://pkg.go.dev/badge/github.com/lignin-dev/lignin.svg)](https://pkg.go.dev/github.com/lignin-dev/lignin)
[![Go Report Card](https://goreportcard.com/badge/github.com/lignin-dev/lignin)](https://goreportcard.com/report/github.com/lignin-dev/lignin)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Go SDK for the Safaricom Daraja API, plus a self-hostable webhook gateway with guaranteed callback delivery, idempotency, and KRA iTax reconciliation exports.

## Install

```bash
go get github.com/lignin-dev/lignin
```

Requires Go 1.22+.

## Usage

```go
client, err := lignin.NewClient(lignin.Config{
    ConsumerKey:    os.Getenv("DARAJA_CONSUMER_KEY"),
    ConsumerSecret: os.Getenv("DARAJA_CONSUMER_SECRET"),
    Environment:    lignin.Sandbox,
})
if err != nil {
    log.Fatal(err)
}

res, err := client.STKPush(ctx, &lignin.STKRequest{
    PhoneNumber: "254712345678",
    Amount:      100,
    AccountRef:  "ORDER-001",
    CallbackURL: "https://your-app.com/mpesa/callback",
})
```

Token refresh, retries, and callback deduplication are handled automatically.

## Supported APIs

- [x] STK Push + query
- [x] C2B (register URLs, simulate)
- [x] B2C payment request
- [x] Account balance
- [x] Transaction status
- [x] Reversal
- [ ] QR code _(planned)_
- [ ] Remittance _(planned)_

## Project Structure

```
lignin/
├── .github/
│   └── workflows/
├── cmd/
│   ├── gateway/
│   └── scheduler/
├── internal/
│   ├── auth/
│   ├── mock/
│   ├── callback/
│   ├── reconciler/
│   ├── scheduler/
│   ├── server/
│   │   └── middleware/
│   ├── store/
│   ├── telemetry/
│   └── errors/
├── migrations/
├── testdata/
│   ├── fixtures/
│   └── golden/
├── scripts/
├── docs/
├── web/
│   └── src/
│       ├── components/
│       ├── composables/
│       └── views/
```

The `cmd/` binaries are thin — all logic lives in `internal/`. The public SDK API is exported from the root package only.

## Gateway

The gateway sits between Daraja and your application. Point your Daraja callback URL here:

```
POST https://hooks.lignin.dev/{endpoint-id}/callback
```

On receipt it verifies the Daraja signature, persists the transaction, deduplicates by `MpesaReceiptNumber`, then forwards a normalised payload to your registered endpoint. Failed deliveries are retried on a cron schedule with exponential backoff.

## Running Locally

**Prerequisites:** Go 1.22+, Docker, Node 20+

```bash
git clone https://github.com/lignin-dev/lignin
cd lignin

# start Postgres + Redis
docker compose up -d

# run gateway
go run ./cmd/gateway

# run scheduler (separate terminal)
go run ./cmd/scheduler

# run dashboard (separate terminal)
cd web && npm install && npm run dev
```

Copy `.env.example` to `.env` and fill in your Daraja sandbox credentials.

```env
DARAJA_CONSUMER_KEY=
DARAJA_CONSUMER_SECRET=
DARAJA_SHORTCODE=174379
DARAJA_PASSKEY=
DARAJA_ENV=sandbox

POSTGRES_DSN=postgres://lignin:lignin@localhost:5432/lignin?sslmode=disable
REDIS_ADDR=localhost:6379
GATEWAY_SECRET=
```

## Testing

```bash
go test ./...
go test ./... -tags=integration   # requires sandbox credentials in .env
```

## Contributing

PRs welcome for uncovered Daraja endpoints, additional test coverage, and reconciliation format support. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

SDK (`lignin.go` and `internal/`) — [MIT](LICENSE).  
Gateway, scheduler, and dashboard — source-available. See [LICENSE-COMMERCIAL](LICENSE-COMMERCIAL).

---

> Not affiliated with Safaricom PLC or M-Pesa.
