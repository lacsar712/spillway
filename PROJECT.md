# Spillway

Hydropower spillway **gate-command relay**: operators send signed gate-move commands; the service interlocks them against reservoir level, then writes to a downstream PLC HTTP endpoint with retry, circuit breaker, rate limit, dead-letter, and replay.

This repository is a **runnable healthy project**. It does not contain planted defects.

## Why this instead of a common business system

This is **plant control-path infrastructure**, not:

- game / graphics
- CLI or desktop file tool
- e-commerce, RBAC, inventory, OA, hospital booking, IM, work-order, parking, auction
- dashboard, weather, habit tracker, player, diary

Product boundary: a signed command enters from the operator desk, passes signature + interlock, and this process is responsible for **writing that command to someone else's PLC HTTP API**. The console only operates the relay (PLC endpoints, attempt journal, replay). It is not a SCADA historian product.

## Roles and happy path

| Role | What they do |
|------|----------------|
| Operator | POST `/api/v1/commands` with HMAC headers and an idempotency key |
| PLC | Registered HTTP URL that accepts a JSON gate-move write |
| Dispatcher | Opens `/` to register PLCs, watch the journal, replay failures, inspect reservoir |

Healthy path:

1. Dispatcher registers a PLC (URL + shared secret + command-type filter).
2. Operator issues `gate.raise` / `gate.lower` / `gate.hold` / `gate.stop`.
3. Relay verifies signature and skew, checks reservoir interlock, de-dupes nonce, fans out to matching PLCs.
4. Worker rate-limits and circuit-breaks, then HTTP POSTs to the PLC; success is journaled; retryable failures back off; terminal failures go to DLQ.
5. Dispatcher replays a dead write from the journal (original body, new delivery id).

## Business rules

### Inbound signature

- `HMAC-SHA256(secret, canonical)` lowercase hex.
- Canonical: `v1.{timestamp}.{nonce}.{sha256_hex(raw_body)}`.
- Headers: `X-Spill-Timestamp`, `X-Spill-Nonce`, `X-Spill-Signature` (`v1=<hex>`), `X-Spill-Source-Key`.
- Window default ±300s. Same nonce only succeeds once inside the window.

### Interlock

`internal/interlock` runs on the **command accept path** after JSON parse:

- `gate.raise` rejected when reservoir level is below spillway crest (nothing to spill).
- `gate.lower` rejected at or above flood stage (must keep routing).
- Bay envelope: opening vs max travel, lockout, 45-minute travel cap.

### Idempotency

- `Idempotency-Key` required (8–128 chars).
- Same key + same body hash: return the first accept, do not enqueue again.
- Same key + different body hash: 409.
- TTL default 24h.

### PLC write

- POST JSON, signed with the **PLC** secret.
- Extra headers: command id, delivery id, attempt, PLC id.
- 2xx success. Retryable: 408, 429, 5xx, network, timeout. Other 4xx terminal → DLQ.

### Retry / circuit / rate limit

- Full-jitter exponential backoff, default 8 attempts.
- Per-PLC breaker: closed → open after fail threshold; half-open probes; **success clears failure count**.
- Per-PLC token bucket; empty bucket delays without burning an attempt.

## HTTP API

Base: `http://127.0.0.1:8080`

- `GET /api/v1/healthz`
- `GET /api/v1/meta`
- `GET/POST /api/v1/plcs`
- `POST /api/v1/plcs/{id}/enable`
- `GET/POST /api/v1/reservoir`
- `GET /api/v1/bays`
- `GET /api/v1/journal`
- `GET /api/v1/dlq`
- `POST /api/v1/replay/{delivery_id}`
- `POST /api/v1/commands` (signed)
- `POST /api/v1/loopback` (seeded PLC target)

## Run

```text
set GOTOOLCHAIN=local
set CGO_ENABLED=0
go test ./...
go run ./cmd/spillway
```

Open http://127.0.0.1:8080/

Default ops secret: `dev-ops-secret`.

## Constraints

- Module `github.com/lacsar712/spillway`, `go 1.22`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`
- Standard library only
- No Docker in this tree
