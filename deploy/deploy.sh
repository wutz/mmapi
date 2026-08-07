#!/bin/bash
set -euo pipefail

HOST=${1:?Usage: deploy.sh <host>}
BINARY=${2:-./mmapi}
# Default to the operator's own filled-in config rather than the sample, which
# by design carries placeholders and is rejected below.
CONFIG=${3:-./config.local.json}
SAMPLE=./deploy/config.sample.json

if [ ! -f "$CONFIG" ]; then
    echo "error: ${CONFIG} does not exist." >&2
    cat >&2 <<EOF

Create it from the sample, filling in the real GUI password and a generated
admin token:

    cp ${SAMPLE} ${CONFIG}
    openssl rand -hex 32   # use for adminToken
EOF
    exit 1
fi

# The sample ships placeholder credentials. mmapi refuses to start on one, so
# pushing it would replace a working config on the node with a config that
# cannot come back up. Catch it here, before anything is overwritten.
if grep -q CHANGE_ME "$CONFIG"; then
    echo "error: ${CONFIG} still holds placeholder credentials:" >&2
    grep -n CHANGE_ME "$CONFIG" >&2
    cat >&2 <<EOF

Fill in the real GUI password and a generated admin token before deploying:

    openssl rand -hex 32   # use for adminToken
EOF
    exit 1
fi

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
