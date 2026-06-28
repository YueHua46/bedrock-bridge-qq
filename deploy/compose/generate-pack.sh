#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p packs
docker compose exec -T bridge pack generate /app/packs/mcqq-bridge-behavior-pack.mcpack
echo "Generated: $(pwd)/packs/mcqq-bridge-behavior-pack.mcpack"
