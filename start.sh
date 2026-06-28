#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
if [[ -x ./mcqq-bridge ]]; then
  ./mcqq-bridge start
else
  echo "mcqq-bridge binary not found, trying go run..."
  go run ./cmd/mcqq-bridge start
fi
