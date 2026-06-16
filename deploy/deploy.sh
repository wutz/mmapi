#!/bin/bash
set -euo pipefail

HOST=${1:?Usage: deploy.sh <host>}
BINARY=${2:-./mmapi}
CONFIG=${3:-./deploy/config.json}

echo "Deploying mmapi to ${HOST}..."

scp "$BINARY" "${HOST}:/usr/local/bin/mmapi"
ssh "$HOST" "chmod +x /usr/local/bin/mmapi"

ssh "$HOST" "mkdir -p /etc/mmapi /var/lib/mmapi"
scp "$CONFIG" "${HOST}:/etc/mmapi/config.json"
scp ./deploy/mmapi.service "${HOST}:/etc/systemd/system/mmapi.service"

ssh "$HOST" "systemctl daemon-reload && systemctl enable mmapi && systemctl restart mmapi"
ssh "$HOST" "systemctl status mmapi --no-pager"

echo "Done. mmapi running on ${HOST}:8080"
