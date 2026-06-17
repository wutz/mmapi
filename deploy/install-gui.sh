#!/bin/bash
set -euo pipefail

# Install and configure GPFS GUI service on a GPFS node
# Usage: install-gui.sh <host> <admin-password>

HOST=${1:?Usage: install-gui.sh <host> <admin-password>}
PASSWORD=${2:?Usage: install-gui.sh <host> <admin-password>}

echo "=== Installing GPFS GUI on ${HOST} ==="

ssh "$HOST" bash <<REMOTE
set -euo pipefail

# Install Java dependency
dpkg -i /usr/lpp/mmfs/*/gpfs_debs/gpfs.java_*.deb 2>/dev/null || true

# Update repos and fix dependencies
apt-get update -qq
apt-get install -y -f -qq

# Install GUI package
dpkg -i /usr/lpp/mmfs/*/gpfs_debs/gpfs.gui_*.deb 2>/dev/null || true
apt-get install -y -f -qq

# Start GUI service
systemctl enable gpfsgui
systemctl start gpfsgui

# Wait for startup
echo "Waiting for GUI to start..."
sleep 20

# Initialize and create admin user
/usr/lpp/mmfs/gui/cli/initgui
/usr/lpp/mmfs/gui/cli/mkuser admin -p '${PASSWORD}' -g 'Administrator,CsiAdmin,ContainerOperator' -e 1 2>/dev/null || \
  /usr/lpp/mmfs/gui/cli/chuser admin -p '${PASSWORD}' -g 'Administrator,CsiAdmin,ContainerOperator' 2>/dev/null || true

echo "GUI service status:"
systemctl status gpfsgui --no-pager | head -5
REMOTE

echo "=== Done. GUI running on ${HOST}:443 ==="
