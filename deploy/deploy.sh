#!/bin/bash
set -euo pipefail

HOST=${1:?Usage: deploy.sh <host>}
BINARY=${2:-./mmapi}
CONFIG=${3:-./deploy/config.json}

echo "Deploying mmapi to ${HOST}..."

# Copy to a staging path first: overwriting a running binary in place fails
# with ETXTBSY ("text file busy") on an upgrade.
scp "$BINARY" "${HOST}:/tmp/mmapi.new"
ssh "$HOST" "install -m 0755 /tmp/mmapi.new /usr/local/bin/mmapi && rm -f /tmp/mmapi.new"

ssh "$HOST" "mkdir -p /etc/mmapi /var/lib/mmapi"
scp "$CONFIG" "${HOST}:/etc/mmapi/config.json"
scp ./deploy/mmapi.service "${HOST}:/etc/systemd/system/mmapi.service"

ssh "$HOST" "systemctl daemon-reload && systemctl enable mmapi && systemctl restart mmapi"
ssh "$HOST" "systemctl status mmapi --no-pager"

echo "Done. mmapi running on ${HOST} (see ${CONFIG} for the port)"
