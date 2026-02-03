#!/usr/bin/env bash
set -euo pipefail

PEERS="${PEERS:-10}"
DURATION_SEC="${DURATION_SEC:-30}"
RATE="${RATE:-10}"
SEED="${SEED:-5}"

go run ./cmd/sim -n "$PEERS" -duration "${DURATION_SEC}s" -rate "$RATE" -seed "$SEED" -drop-rate 0 -pow-invalid-rate 0
