#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created .env. Please edit BRIDGE_PUBLIC_URL and QQ_GROUP_ID, then run ./init.sh again."
  exit 1
fi

set -a
source .env
set +a

mkdir -p data logs packs napcat/config napcat/qq napcat/plugins

echo "Building and starting services..."
docker compose up -d --build

echo "Initializing Bridge config..."
docker compose exec -T bridge init
docker compose exec -T bridge config set onebot.ws_url ws://napcat:3001
docker compose exec -T bridge config set onebot.http_url http://napcat:3000

if [[ -n "${BRIDGE_PUBLIC_URL:-}" && "${BRIDGE_PUBLIC_URL}" != "http://YOUR_SERVER_IP:8080" ]]; then
  docker compose exec -T bridge config set server.public_url "${BRIDGE_PUBLIC_URL}"
else
  echo "WARN: BRIDGE_PUBLIC_URL is still the placeholder. Edit .env before generating the final behavior pack."
fi

if [[ -n "${QQ_GROUP_ID:-}" && "${QQ_GROUP_ID}" != "123456789" ]]; then
  docker compose exec -T bridge config set qq.group_id "${QQ_GROUP_ID}"
else
  echo "WARN: QQ_GROUP_ID is still the placeholder. Edit .env before real use."
fi

docker compose exec -T bridge config set qq.forward_prefix "${QQ_FORWARD_PREFIX:-}"

if [[ -n "${ONEBOT_ACCESS_TOKEN:-}" ]]; then
  docker compose exec -T bridge config set onebot.access_token "${ONEBOT_ACCESS_TOKEN}"
fi

echo "Generating behavior pack..."
docker compose exec -T bridge pack generate /app/packs/mcqq-bridge-behavior-pack.mcpack

echo "Restarting Bridge to load the saved config..."
docker compose restart bridge

echo
echo "Done."
echo "Bridge Web UI:        ${BRIDGE_PUBLIC_URL:-http://YOUR_SERVER_IP:8080}/setup"
echo "NapCat WebUI:         http://YOUR_SERVER_IP:6099/webui"
echo "Behavior pack path:   $(pwd)/packs/mcqq-bridge-behavior-pack.mcpack"
echo
echo "Next:"
echo "1. Open NapCat WebUI and log in with QQ."
echo "2. Copy packs/mcqq-bridge-behavior-pack.mcpack to your BDS world and enable it."
echo "3. View logs with: ./logs.sh"
