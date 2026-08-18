# Spillway

Signed hydropower **gate-command** relay: reservoir interlock, then reliable HTTP writes to a downstream PLC (retry, circuit breaker, rate limit, DLQ, replay).

Design: [PROJECT.md](PROJECT.md).

## Run

```text
set GOTOOLCHAIN=local
set CGO_ENABLED=0
go test ./...
go run ./cmd/spillway
```

Open http://127.0.0.1:8080/

Default operator secret: `dev-ops-secret`. A loopback PLC is seeded so a test `gate.raise` can succeed without an external controller.
