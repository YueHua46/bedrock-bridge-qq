#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p dist
go mod tidy
export CGO_ENABLED=0

GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/mcqq-bridge-windows-amd64.exe ./cmd/mcqq-bridge
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/mcqq-bridge-linux-amd64 ./cmd/mcqq-bridge
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/mcqq-bridge-linux-arm64 ./cmd/mcqq-bridge
