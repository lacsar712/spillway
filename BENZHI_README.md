# Spillway

Signed hydropower gate-command relay: reservoir interlock, then reliable HTTP writes to a downstream PLC (retry, circuit breaker, rate limit, DLQ, replay).

## Requirements

- Go 1.22+ (container image uses golang:1.22)
- `GOTOOLCHAIN=local` recommended on host when using a pinned toolchain

## Build

```bash
export GOTOOLCHAIN=local
go build ./...
```

## Run

```bash
export GOTOOLCHAIN=local
go run ./cmd/spillway
```

Open http://127.0.0.1:8080/

Default operator secret: `dev-ops-secret`.

## Test

```bash
export GOTOOLCHAIN=local
go test ./... -count=1
```

## Docker (benzhi)

Must build **linux/amd64** and **linux/arm64**:

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh spillway linux/amd64
./build_benzhi_docker.sh spillway linux/arm64
docker run -it spillway:latest
# inside container:
export GOTOOLCHAIN=local
go version
go build ./...
go test ./... -count=1
```