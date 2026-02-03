param(
  [int]$SimPeers = 5,
  [int]$SimDurationSec = 5,
  [int]$SimRate = 20
)

$ErrorActionPreference = 'Stop'

Write-Host '== go test =='
go test ./...

Write-Host '== go vet =='
go vet ./...

Write-Host '== go build =='
go build ./...

Write-Host '== golangci-lint =='
if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) {
  throw 'golangci-lint not found in PATH'
}
golangci-lint run

Write-Host '== sim A =='
go run ./cmd/sim -n $SimPeers -duration "${SimDurationSec}s" -rate $SimRate -seed 1 -drop-rate 0 -pow-invalid-rate 0

Write-Host '== sim B =='
go run ./cmd/sim -n $SimPeers -duration "${SimDurationSec}s" -rate $SimRate -seed 2 -drop-rate 0.2 -pow-invalid-rate 0

Write-Host '== sim C =='
go run ./cmd/sim -n $SimPeers -duration "${SimDurationSec}s" -rate $SimRate -seed 3 -drop-rate 0 -pow-invalid-rate 0.3

Write-Host '== sim v2 =='
go run ./cmd/sim -profile v2 -n 10 -seed 1
