# mmapi

GPFS multi-tenant API proxy for CSI support. Transparently proxies the IBM Storage Scale GUI REST API (`/scalemgmt/v2/`) with token-based per-filesystem access control.

## Architecture

```
CSI Driver → mmapi (token auth + filesystem access control) → GPFS GUI (real API) → GPFS Cluster
```

mmapi does NOT implement any GPFS commands. It is a pure reverse proxy that:
1. Authenticates requests using mmapi tokens (via Basic Auth)
2. Checks if the token has access to the requested filesystem
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
  -H 'Authorization: Bearer <adminToken>' \
  -H 'Content-Type: application/json' \
  -d '{"allowedFs":["fs0"]}'

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
  "guiPassword": "Admin@123",
  "adminToken": "mmapi_admin_ChangeMe_2026",
  "guiVerifyTLS": false
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `port` | HTTP(S) listen port | 8443 |
| `dataDir` | Token and TLS cert storage | `/var/lib/mmapi` |
| `tls` | Enable HTTPS with self-signed cert | `true` |
| `certFile` | TLS cert path (auto-generated if unset) | `<dataDir>/tls.crt` |
| `keyFile` | TLS key path (auto-generated if unset) | `<dataDir>/tls.key` |
| `guiUrl` | Upstream GPFS GUI URL | (required) |
| `guiUsername` | GUI admin username | (required) |
| `guiPassword` | GUI admin password | (required) |
| `adminToken` | Bearer token protecting the token management API (empty = open) | (empty) |
| `guiVerifyTLS` | Verify the upstream GUI TLS certificate | `false` |

> ⚠️ Set a strong `adminToken` before exposing the management API. When empty, `/api/v1/tokens` is unauthenticated.

## API

### Proxied Endpoints (Scale GUI compatible)

All `/scalemgmt/v2/` requests are proxied to the GPFS GUI. Authentication uses Basic Auth where the password is an mmapi token.

### Access Control

Tokens restrict access at filesystem granularity (`allowedFs`): only requests
targeting an allowed filesystem are forwarded. A token that owns a filesystem
owns every fileset inside it, including filesets the CSI driver provisions
dynamically (`pvc-<uuid>`).

Isolate tenants by giving each one its own filesystem. Sharing a single
filesystem between tenants is not a supported isolation boundary — fileset-level
access control was removed because dynamically provisioned fileset names cannot
be allowlisted in advance.

### Token Management

All token management endpoints require a `Bearer` admin token (the `adminToken`
config field), e.g. `-H "Authorization: Bearer <adminToken>"`.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/tokens` | Create token |
| GET | `/api/v1/tokens` | List tokens (secret omitted) |
| DELETE | `/api/v1/tokens/{id}` | Delete token |

**Create token request body:**
```json
{
  "allowedFs": ["fs0"]
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
mmctl quota list <target> [type]       # List quotas (type: USR|GRP|FILESET)
mmctl quota set <fs> <fset> <soft> <hard>  # Set fileset quota
mmctl quota user list <target>         # List user quotas
mmctl quota user set <target> <user> <soft> <hard> [<filesSoft> <filesHard>]
mmctl quota user unset <target> <user> # Remove a user quota
mmctl quota group list <target>        # List group quotas
mmctl quota group set <target> <group> <soft> <hard> [<filesSoft> <filesHard>]
mmctl quota group unset <target> <group>   # Remove a group quota
mmctl token create <fs1,fs2>           # Create token
mmctl token list                       # List tokens
mmctl token delete <id>                # Delete token
```

### Quotas

Quota commands take a target in the `mmsetquota` `Device[:Fileset]` form:
`fs0` addresses the filesystem, `fs0:fset1` addresses one fileset inside it.

Which one to use is decided by the filesystem, not by preference. When
`mmlsfs <fs> --perfileset-quota` reports `yes` — the default for filesystems
serving CSI — user and group quotas exist per fileset only, and the GUI rejects
filesystem-scoped writes with *"Per fileset quota enabled on this filesystem"*.
Use `fs0:fset1` there. Fileset quotas themselves are always filesystem-scoped
(`mmctl quota set fs0 fset1 ...`).

Limits use GPFS syntax (`10G`, `512M`, `1T`); `0` means unlimited. Block and
file (inode) limits are both supported — pass the optional
`<filesSoft> <filesHard>` pair to set inode limits alongside block limits.
GPFS has no delete-quota operation, so `unset` zeroes every limit.

Quota writes are asynchronous in the GUI: it answers `202` with a job handle.
mmctl follows the job to completion and reports `mmsetquota`'s own output, so a
failure surfaces as a non-zero exit rather than a silent accept.

Reads go through the GUI's own quota cache, which it refreshes a few seconds
after each write. A `quota list` issued immediately after a `set` can therefore
still show the previous limits; re-run it, or use `mmlsquota` on the cluster for
the authoritative value. This is GUI behaviour, not proxy caching — mmapi
forwards every request untouched.

```bash
# Per-fileset user and group quotas (per-fileset quota enabled)
mmctl quota user set fs0:fset1 ubuntu 1G 2G
mmctl quota group set fs0:fset1 ubuntu 1G 2G 1000 2000
mmctl quota user list fs0:fset1
mmctl quota user unset fs0:fset1 ubuntu

# Filesystem-wide user quota (per-fileset quota disabled)
mmctl quota user set fs0 ubuntu 1G 2G
```

Tokens authorize at filesystem granularity, so a token holding `fs0` may manage
user and group quotas anywhere inside `fs0`.

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
  -H 'Authorization: Bearer <adminToken>' \
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
