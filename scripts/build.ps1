param(
  [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
New-Item -ItemType Directory -Force -Path dist | Out-Null

go mod tidy
$env:CGO_ENABLED = "0"

$targets = @(
  @{ GOOS="windows"; GOARCH="amd64"; Out="dist/mcqq-bridge-windows-amd64.exe" },
  @{ GOOS="linux"; GOARCH="amd64"; Out="dist/mcqq-bridge-linux-amd64" },
  @{ GOOS="linux"; GOARCH="arm64"; Out="dist/mcqq-bridge-linux-arm64" }
)

foreach ($t in $targets) {
  $env:GOOS = $t.GOOS
  $env:GOARCH = $t.GOARCH
  go build -trimpath -ldflags "-s -w" -o $t.Out ./cmd/mcqq-bridge
  Write-Host "built $($t.Out)"
}
