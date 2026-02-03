#!/usr/bin/env bash
set -euo pipefail

SIM_PEERS="${SIM_PEERS:-5}"
SIM_DURATION_SEC="${SIM_DURATION_SEC:-5}"
SIM_RATE="${SIM_RATE:-20}"

printf '== go test ==\n'
go test ./...

printf '== go vet ==\n'
go vet ./...

printf '== go build ==\n'
go build ./...

printf '== golangci-lint ==\n'
command -v golangci-lint >/dev/null 2>&1 || { echo 'golangci-lint not found in PATH' >&2; exit 1; }
golangci-lint run

printf '== sim A ==\n'
go run ./cmd/sim -n "$SIM_PEERS" -duration "${SIM_DURATION_SEC}s" -rate "$SIM_RATE" -seed 1 -drop-rate 0 -pow-invalid-rate 0

printf '== sim B ==\n'
go run ./cmd/sim -n "$SIM_PEERS" -duration "${SIM_DURATION_SEC}s" -rate "$SIM_RATE" -seed 2 -drop-rate 0.2 -pow-invalid-rate 0

printf '== sim C ==\n'
go run ./cmd/sim -n "$SIM_PEERS" -duration "${SIM_DURATION_SEC}s" -rate "$SIM_RATE" -seed 3 -drop-rate 0 -pow-invalid-rate 0.3

printf '== sim v2 ==\n'
go run ./cmd/sim -profile v2 -n 10 -seed 1
