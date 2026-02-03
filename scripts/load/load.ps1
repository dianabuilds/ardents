param(
  [int]$Peers = 50,
  [int]$DurationSec = 10,
  [int]$Rate = 50,
  [int]$Seed = 4
)

$ErrorActionPreference = 'Stop'

go run ./cmd/sim -n $Peers -duration "${DurationSec}s" -rate $Rate -seed $Seed -drop-rate 0 -pow-invalid-rate 0
