param(
  [string]$OutDir = "dist",
  [string]$PackageName = "MCQQ-Bridge-Windows-x64"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$out = Join-Path $root $OutDir
$stage = Join-Path $out $PackageName
$downloads = Join-Path $out "downloads"

Set-Location $root
New-Item -ItemType Directory -Force -Path $out, $downloads | Out-Null

Write-Host "Building mcqq-bridge.exe..."
go mod tidy
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w" -o (Join-Path $out "mcqq-bridge.exe") ./cmd/mcqq-bridge

Write-Host "Downloading latest NapCat Windows OneKey package..."
$headers = @{ "User-Agent" = "MCQQ-Bridge-Packager" }
$release = Invoke-RestMethod -Headers $headers -Uri "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest"
$assets = Invoke-RestMethod -Headers $headers -Uri $release.assets_url
$asset = $assets | Where-Object { $_.name -eq "NapCat.Shell.Windows.OneKey.zip" } | Select-Object -First 1
if (-not $asset) {
  throw "NapCat.Shell.Windows.OneKey.zip was not found in latest NapCat release."
}
$napcatZip = Join-Path $downloads "NapCat.Shell.Windows.OneKey.zip"
Invoke-WebRequest -Headers $headers -Uri $asset.browser_download_url -OutFile $napcatZip

Write-Host "Assembling package..."
if (Test-Path $stage) {
  Remove-Item -LiteralPath $stage -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $stage, (Join-Path $stage "data"), (Join-Path $stage "logs"), (Join-Path $stage "napcat") | Out-Null

Copy-Item -LiteralPath (Join-Path $out "mcqq-bridge.exe") -Destination $stage -Force
Copy-Item -LiteralPath (Join-Path $root "start.bat"), (Join-Path $root "stop.bat"), (Join-Path $root "update.bat"), (Join-Path $root "README.md") -Destination $stage -Force
Expand-Archive -LiteralPath $napcatZip -DestinationPath (Join-Path $stage "napcat") -Force
Copy-Item -LiteralPath (Join-Path $root "scripts" "start-napcat.bat") -Destination (Join-Path $stage "napcat" "start-napcat.bat") -Force
Copy-Item -LiteralPath (Join-Path $root "scripts" "README-NapCat.txt") -Destination (Join-Path $stage "napcat" "README-MCQQ.txt") -Force

$zipPath = Join-Path $out "$PackageName.zip"
if (Test-Path $zipPath) {
  Remove-Item -LiteralPath $zipPath -Force
}
Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zipPath -Force
Write-Host "Package created: $zipPath"
