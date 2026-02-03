param(
  [int]$Peers = 10,
  [int]$DurationSec = 30,
  [int]$Rate = 10,
  [int]$Seed = 5
)

$ErrorActionPreference = 'Stop'

go run ./cmd/sim -n $Peers -duration "${DurationSec}s" -rate $Rate -seed $Seed -drop-rate 0 -pow-invalid-rate 0
