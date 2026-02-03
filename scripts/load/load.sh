#!/usr/bin/env bash
set -euo pipefail

PEERS="${PEERS:-50}"
DURATION_SEC="${DURATION_SEC:-10}"
RATE="${RATE:-50}"
SEED="${SEED:-4}"

go run ./cmd/sim -n "$PEERS" -duration "${DURATION_SEC}s" -rate "$RATE" -seed "$SEED" -drop-rate 0 -pow-invalid-rate 0
