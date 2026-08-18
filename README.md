# WaferLot

Semiconductor FAB lot-step event relay. Tools post signed lot / step / tool events; the service journals them and forwards to a manufacturing execution HTTP collector with retry, isolator, rate limit, hold bin, and reissue.

Full design: [PROJECT.md](PROJECT.md).

## Run

```text
set GOTOOLCHAIN=local
set CGO_ENABLED=0
go test ./...
go run ./cmd/waferlot
```

Open http://127.0.0.1:8080/

Default tool ingest secret: `dev-tool-secret`. A seeded echo collector lets a test lot event succeed without an external MES URL.
