# mmapi

GPFS multi-tenant API proxy for CSI support. Transparently proxies the IBM Storage Scale GUI REST API (`/scalemgmt/v2/`) with token-based per-filesystem and per-fileset access control.

## Architecture

```
CSI Driver → mmapi (token auth + FS/fileset access control) → GPFS GUI (real API) → GPFS Cluster
```

mmapi does NOT implement any GPFS commands. It is a pure reverse proxy that:
1. Authenticates requests using mmapi tokens (via Basic Auth)
2. Checks if the token has access to the requested filesystem and fileset
3. Forwards the request to the real GPFS GUI with admin credentials
4. Returns the GUI's response unmodified

## Quick Start

```bash
# Build
make build-linux

# Deploy GPFS GUI (if not installed)
./deploy/install-gui.sh root@<gpfs-node> <admin-password>

# Deploy mmapi proxy
./deploy/deploy.sh root@<gpfs-node> ./mmapi ./deploy/config-owning.json

# Create an access token (filesystem-level)
curl -sk -X POST https://<host>:8443/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"allowedFs":["fs0"]}'

# Create a token with fileset restriction (optional)
curl -sk -X POST https://<host>:8443/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"allowedFs":["fs0"],"allowedFileset":["pvc-xxx","pvc-yyy"]}'

# Test via mmctl
export MMAPI_URL=https://<host>:8443
export MMAPI_TOKEN=<token-secret>
mmctl fs list
mmctl fileset list fs0
```

## Configuration

`/etc/mmapi/config.json`:

```json
{
  "port": 8443,
  "dataDir": "/var/lib/mmapi",
  "tls": true,
  "guiUrl": "https://127.0.0.1:443",
  "guiUsername": "admin",
  "guiPassword": "Admin@123"
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `port` | HTTP(S) listen port | 8443 |
| `dataDir` | Token and TLS cert storage | `/var/lib/mmapi` |
| `tls` | Enable HTTPS with self-signed cert | `true` |
| `guiUrl` | Upstream GPFS GUI URL | (required) |
| `guiUsername` | GUI admin username | (required) |
| `guiPassword` | GUI admin password | (required) |

## API

### Proxied Endpoints (Scale GUI compatible)

All `/scalemgmt/v2/` requests are proxied to the GPFS GUI. Authentication uses Basic Auth where the password is an mmapi token.

### Access Control

Tokens restrict access at two levels:
- **Filesystem level** (`allowedFs`): Only requests targeting allowed filesystems are forwarded
- **Fileset level** (`allowedFileset`, optional): Only requests targeting allowed filesets are forwarded. If empty, all filesets in allowed filesystems are accessible.

### Token Management

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/tokens` | Create token |
| GET | `/api/v1/tokens` | List tokens |
| DELETE | `/api/v1/tokens/{id}` | Delete token |

**Create token request body:**
```json
{
  "allowedFs": ["fs0"],
  "allowedFileset": ["pvc-xxx", "pvc-yyy"]
}
```

## mmctl CLI

```bash
mmctl cluster                          # Cluster info
mmctl fs list                          # List filesystems
mmctl fs get <name>                    # Get filesystem details
mmctl fileset list <fs>                # List filesets
mmctl fileset create <fs> <name>       # Create fileset
mmctl fileset delete <fs> <name>       # Delete fileset
mmctl fileset link <fs> <name> <path>  # Link fileset
mmctl fileset unlink <fs> <name>       # Unlink fileset
mmctl quota list <fs>                  # List quotas
mmctl quota set <fs> <fset> <soft> <hard>  # Set quota
mmctl token create <fs1,fs2>           # Create token
mmctl token list                       # List tokens
mmctl token delete <id>                # Delete token
```

## GPFS CSI Deployment

### Prerequisites

1. GPFS GUI installed and running on cluster nodes
2. mmapi deployed as proxy
3. GUI admin user with roles: Administrator, CsiAdmin, ContainerOperator

### Install

```bash
# 1. Deploy GPFS GUI
./deploy/install-gui.sh root@<gpfs-node> <password>

# 2. Deploy mmapi
./deploy/deploy.sh root@<gpfs-node> ./mmapi ./deploy/config-owning.json

# 3. Create mmapi token for CSI
curl -sk -X POST https://<host>:8443/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"allowedFs":["fs0"]}'

# 4. Install CSI driver
# Edit deploy/csi/secrets.yaml with your token
# Edit deploy/csi/csiscaleoperator.yaml with your cluster details
./deploy/csi/install.sh

# 5. Create StorageClass and test
kubectl apply -f deploy/csi/storageclass.yaml
kubectl apply -f deploy/csi/example-pod.yaml
```

### CSI Manifests

| File | Description |
|------|-------------|
| `deploy/csi/install.sh` | CSI operator installation script |
| `deploy/csi/secrets.yaml` | Auth secrets |
| `deploy/csi/csiscaleoperator.yaml` | CSI operator CR (cluster, node mapping) |
| `deploy/csi/storageclass.yaml` | GPFS fileset StorageClass |
| `deploy/csi/example-pod.yaml` | Example Pod + PVC |

## Development

```bash
make build    # Build mmapi + mmctl
make test     # Run tests
```
