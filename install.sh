#!/usr/bin/env bash
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$APP_DIR/mcqq-bridge"

if [[ ! -x "$BIN" ]]; then
  echo "mcqq-bridge binary not found or not executable: $BIN"
  echo "Build it first: go build -o mcqq-bridge ./cmd/mcqq-bridge"
  exit 1
fi

"$BIN" init

if command -v systemctl >/dev/null 2>&1 && [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  cat >/etc/systemd/system/mcqq-bridge.service <<EOF
[Unit]
Description=MCQQ Bridge
After=network.target

[Service]
Type=simple
WorkingDirectory=$APP_DIR
ExecStart=$BIN start
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now mcqq-bridge
  echo "MCQQ Bridge installed as systemd service."
  systemctl status mcqq-bridge --no-pager || true
else
  echo "systemd service was not installed. Start manually with: ./start.sh"
fi

echo "Open the setup page from your browser:"
echo "  http://127.0.0.1:8080/setup"
