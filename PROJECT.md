# WaferLot

FAB lot-step event relay: tools post signed lot / step / tool events; this process journals each attempt and forwards JSON to a manufacturing execution HTTP collector, with retry, isolator, rate limit, hold bin, and operator reissue.

This repository is a **runnable healthy project**. It does not ship pre-planted defects on `main`.

## 1. Why this, not a common business system

This is **outbound MES forwarding infrastructure**, not:

- Inventory, e-commerce, OA, CRM, ticketing, parking, auctions
- Games / graphics, CLI file tools
- Hospital booking, takeout, IM
- Dashboards, accounting, fitness, cooking recipes, weather, pomodoro, habit trackers

Product boundary: a tool posts a signed lot-step event; this process is responsible for **delivering it to someone else's MES HTTP collector**. The operator console only operates forwarding itself (collectors, run log, hold bin, reissue). Process programs are called **process program**, never cooking recipes.

## 2. Roles and happy path

| Role | What they do |
|------|----------------|
| Tool | POST `/api/v1/lots` with HMAC headers, nonce, and idempotency key |
| MES collector | Registered HTTP URL that receives the lot-step JSON |
| Operator | Opens `/` to register collectors, inspect the run log, and reissue hold-bin items |

Happy path:

1. Operator registers a collector (URL + shared secret + lot-event kind filter).
2. A tool posts a lot event (kind, payload with lot / tool / step / process program).
3. The relay verifies signature and time window, dedupes nonce, matches collectors, and enqueues a forward.
4. The forwarder POSTs with backoff; success is journaled; retryable failures requeue; terminal failures enter the hold bin.
5. The operator sees attempts on the console; failed items can be reissued.

## 3. Rules that must be implemented

### 3.1 Inbound signature

- Algorithm: `HMAC-SHA256(secret, canonical)`, lowercase hex.
- Canonical string: `v1.{timestamp}.{nonce}.{sha256_hex(raw_body)}`.
- Headers:
  - `X-Wafer-Timestamp`: Unix seconds
  - `X-Wafer-Nonce`: 16–64 printable bytes
  - `X-Wafer-Signature`: `v1=<hex>`
  - `X-Wafer-Tool-Key`: ingest key id (default `tool`)
- Time window: default ±300 seconds.
- The same nonce may succeed only once inside the window.
- Tool ingest secrets are separate from collector outbound secrets.

### 3.2 Idempotency

- Caller must send `Idempotency-Key` (8–128 characters).
- Same key + same body hash: return the first accept result, do not enqueue again.
- Same key + different body hash: 409 Conflict.
- Records expire after TTL (default 24h).

### 3.3 Collectors and fan-out

- A collector has: id, name, URL, outbound secret, enabled flag, kind prefixes, per-collector in-flight, token bucket.
- A lot event kind hits a prefix only on a **segment boundary** (`lot` matches `lot.track_in`, not `lotx.track_in`).
- One inbound event may fan out to several collectors; each has an independent wait line and run-log row.
- `ordered=true` serializes forwards on that collector.

### 3.4 Outbound request

- POST, `Content-Type: application/json`.
- Outbound signature uses the **collector** secret.
- Extra headers: `X-Wafer-Lot-Id`, `X-Wafer-Forward-Id`, `X-Wafer-Attempt`, `X-Wafer-Collector`.
- Timeout default 10s; do not follow cross-host 3xx.
- 2xx is success.
- Retryable: 408, 429, 500–599, network error, timeout.
- Terminal: 400–407, 409–428, 430–499 (including 422) → hold bin.

### 3.5 Retry

- Full jitter: `sleep = random(0, min(cap, base * 2^attempt))`.
- Default `base=200ms`, `cap=30s`, `maxAttempts=8` (including the first try).
- Attempt numbers in the run log start at 1.

### 3.6 Isolator (circuit)

Per collector:

- Closed: consecutive failures ≥ `failThreshold` (default 5) → Open.
- Open: reject new attempts for `openFor` (default 30s), then HalfOpen.
- HalfOpen: allow `probe` (default 1) probes; success → Closed and clear count; failure → Open again.

While open, work stays in the wait line and is journaled as `skipped_open` (not a business failure attempt).

### 3.7 Rate limit

Per-collector token bucket: capacity `burst`, rate `rate` tokens/sec. No token → delay and retry without consuming an attempt.

### 3.8 Hold bin and reissue

- Terminal outcome or attempts exhausted → hold bin.
- Reissue: take original body + collector from the run log, mint a new `forward_id`, reset attempt to 1. Old idempotency keys do not block internal reissue.

### 3.9 Payload and snapshot

- Body max 256 KiB.
- Console lists mask `authorization` / `password` / `secret` / `token` fields; storage keeps the original for reissue.
- Snapshot memory to the data directory on a ticker and on shutdown.

## 4. HTTP API

Base: `http://127.0.0.1:8080`

Control plane (unsigned, local demo):

- `GET /api/v1/healthz`
- `GET /api/v1/meta`
- `GET|POST /api/v1/collectors`
- `POST /api/v1/collectors/{id}/enable`
- `GET /api/v1/runlog?collector_id=&limit=`
- `GET /api/v1/holdbin`
- `POST /api/v1/reissue/{forward_id}`
- `GET /api/v1/echo/recent`
- `GET /api/v1/trips`
- `GET /api/v1/tools`
- `GET /api/v1/steps`
- `GET /api/v1/programs`

Data plane:

- `POST /api/v1/lots` — signed lot-step event
- `POST /api/v1/echo` — seeded collector target

## 5. Operator UI

Static `web/index.html` + `web/app.js` + `web/style.css` at `/`.

## 6. Run

```text
set GOTOOLCHAIN=local
set CGO_ENABLED=0
go test ./...
go run ./cmd/waferlot
```

| Variable | Default |
|----------|---------|
| `WAFERLOT_ADDR` | `:8080` |
| `WAFERLOT_DATA_DIR` | `./data` |
| `WAFERLOT_INGEST_SECRET` | `dev-tool-secret` |
| `WAFERLOT_WINDOW_SEC` | `300` |

## 7. Constraints

- Module: `github.com/lacsar712/waferlot`
- `go.mod` language `1.22`; run with `GOTOOLCHAIN=local`
- No CGO; snapshot storage is pure Go
- Standard library only
- No Docker in this repository
