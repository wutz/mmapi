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

# Deploy mmapi proxy (config.local.json is your filled-in copy of the sample;
# see Configuration)
./deploy/deploy.sh root@<gpfs-node> ./mmapi ./config.local.json

# Create an access token (filesystem-level).
# MMAPI_ADMIN_TOKEN is the adminToken from config.json, and reaches only the
# three mm*token commands.
export MMAPI_URL=https://<host>:8443
export MMAPI_ADMIN_TOKEN=<adminToken>
mmctl mmcrtoken fs0
# Token 4f8c1e0b7a2d9354 created successfully.
# Secret: mmapi_...

# Test via mmctl, which speaks the GPFS mm* commands. These authenticate with
# the access token just printed, never with the admin token.
export MMAPI_TOKEN=mmapi_...
mmctl mmlsfs fs0 -T
mmctl mmlsfileset fs0
```

The same token can be created over the API directly; see
[Token Management](#token-management).

## Configuration

`/etc/mmapi/config.json`:

```json
{
  "port": 8443,
  "dataDir": "/var/lib/mmapi",
  "tls": true,
  "guiUrl": "https://127.0.0.1:443",
  "guiUsername": "admin",
  "guiPassword": "<gui-password>",
  "adminToken": "<generated-token>",
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
| `adminToken` | Bearer token protecting the token management API | (required) |
| `guiVerifyTLS` | Verify the upstream GUI TLS certificate | `false` |

`guiUrl`, `guiUsername`, `guiPassword` and `adminToken` are all mandatory, and
mmapi exits at startup if one is missing or still carries the `CHANGE_ME`
placeholder the samples ship — an mmapi without an admin token would let any
caller reach `/api/v1/tokens` and mint a token for any filesystem. Generate one
with `openssl rand -hex 32`.

`deploy/config.json` and `deploy/config-owning.json` are samples, not
deployables. Copy one, fill it in, and deploy the copy; `deploy/deploy.sh`
refuses a config that still holds a placeholder rather than replace a working
config on the node with one the server will reject:

```bash
cp deploy/config-owning.json config.local.json   # git-ignored
openssl rand -hex 32                             # use for adminToken
./deploy/deploy.sh root@<gpfs-node> ./mmapi ./config.local.json
```

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
| DELETE | `/api/v1/tokens/{id}` | Delete token (404 if no token carries that id) |

**Create token request body:**
```json
{
  "allowedFs": ["fs0"]
}
```

## mmctl CLI

mmctl is the GPFS command line, aimed at mmapi instead of a cluster node. Every
command is named after the `mm*` command it stands for, takes that command's
options and operands, and prints what it prints — so `mmctl mmlsquota -j fset1
fs0` produces the same table as `mmlsquota -j fset1 fs0` on the cluster.

```bash
mmctl mmlscluster [-Y]                       # Cluster configuration
mmctl mmlsfs {Device | all} [-T] [-Q] ...    # File system attributes
mmctl mmlsfileset Device [Fileset,...] [-d] [-i] [-L] [-Y]
mmctl mmcrfileset Device FilesetName [-t Comment] [-J JunctionPath]
                  [--inode-space {new | ExistingFileset}] [--inode-limit N]
mmctl mmchfileset Device FilesetName [-j NewName] [-t NewComment]
mmctl mmdelfileset Device FilesetName [-f]
mmctl mmlinkfileset Device FilesetName [-J JunctionPath]
mmctl mmunlinkfileset Device FilesetName [-f]
mmctl mmlsquota [-u User | -g Group | -j Fileset] [-Y]
                [--block-size {BlockSize | auto}] [Device[:Fileset] ...]
mmctl mmrepquota [-u] [-g] [-j] [-n] [-Y] {-a | Device[:Fileset] ...}
mmctl mmsetquota Device[:Fileset] [--user IdOrName[,...]] [--group IdOrName[,...]]
                 [--block SoftLimit[:HardLimit]] [--files SoftLimit[:HardLimit]]
mmctl mmsetquota Device[:Fileset] --default {user | group | fileset} ...
mmctl mmsetquota Device[:Fileset] --grace {user | group | fileset} ...
mmctl mmcrtoken Device[,Device...]           # mmapi extension
mmctl mmlstoken [-Y]                         # mmapi extension
mmctl mmdeltoken TokenId                     # mmapi extension
```

`mmctl <command> --help` prints that command's synopsis; `mmctl help` lists them
all. A symlink named after a command invokes it directly, the way the commands
are reached on a cluster node:

```bash
ln -s /usr/local/bin/mmctl /usr/local/bin/mmlsquota
mmlsquota -j fset1 fs0
```

### Environment

| Variable | Read by | Sent as |
|---|---|---|
| `MMAPI_URL` | every command (default `https://localhost:8443`) | — |
| `MMAPI_TOKEN` | every GPFS command — everything proxied to `/scalemgmt/v2/` | `Authorization: Basic base64(admin:<token>)` |
| `MMAPI_ADMIN_TOKEN` | `mmcrtoken`, `mmlstoken`, `mmdeltoken` only | `Authorization: Bearer <token>` |

**The two tokens are not interchangeable, and the admin token is not the more
privileged of the two.** Access tokens have no GPFS equivalent, so `mmcrtoken`,
`mmlstoken` and `mmdeltoken` are mmapi's own and authenticate with
`MMAPI_ADMIN_TOKEN`; every other command authenticates with `MMAPI_TOKEN` and
ignores `MMAPI_ADMIN_TOKEN` entirely. Export only the admin token and
`mmctl mmlscluster` sends no credentials at all:

```console
$ export MMAPI_ADMIN_TOKEN=mmapi_admin_ChangeMe_2026
$ mmctl mmlscluster
mmlscluster: unauthorized
```

Both variables can be exported at once — mmctl picks the right one per command.

### Troubleshooting

| Message | Cause |
|---|---|
| `unauthorized` | `MMAPI_TOKEN` is unset, or names a token the server does not know. Tokens live in `dataDir` (default `/var/lib/mmapi`); wiping it on redeploy invalidates every secret issued before, so reissue with `mmcrtoken`. |
| `access denied: filesystem "fs0" not allowed` | The token is valid but its `allowedFs` does not list that filesystem. `mmlstoken` shows what each token covers. |
| `connection refused`, `no route to host` | `MMAPI_URL` still points at the default `localhost:8443`. It must name the node running mmapi whenever mmctl runs elsewhere. |
| `admin authentication required` | `MMAPI_ADMIN_TOKEN` is unset or does not match the server's `adminToken`. Only the three `mm*token` commands read it. |
| `token not found` from `mmdeltoken` | No token carries that id — check `mmlstoken`. The secret being revoked is still live. |

Commands that address no filesystem — `mmlscluster` among them — skip the
`allowedFs` check, so any valid token runs them. An `unauthorized` from
`mmlscluster` is therefore always the token itself, never its scope.

### Where mmctl and the cluster differ

mmctl reports what the GUI REST API reports, which is not quite everything the
mm commands read from the local daemon:

| Difference | Detail |
|---|---|
| Missing `mmlsfs` attributes | `--subblocks-per-full-block`, `--maintenance-mode`, `--flush-on-close`, `--auto-inode-limit`, `--nfs4-owner-write-acl`, `--inode-segment-mgr` and the per-role file system versions are not in the API; `-V` prints `37.00` rather than `37.00 (5.2.3.0)` |
| `mmlsfileset -d` | Reports the GUI's usage figure (quota accounting), which lags and differs from the inode scan `mmlsfileset -d` performs on a node |
| `-Y` records | The record and field names match, but fields the API does not report are `-` or empty: `preventSnapshotRestore`, `permInheritFlag` and `falStatus` in `mmlsfileset`, `remarks` in `mmrepquota`, the internal `otherNodeRoles` code in `mmlscluster` (its readable alias is filled) |
| Quota freshness | Reads come from the GUI's quota cache, refreshed a few seconds after a write |
| Missing warnings | The cluster's own advisories, such as *"quota accounting information is outdated. Run mmcheckquota"*, are not surfaced |
| `mmcrfileset` | The GUI links the new fileset immediately, where `mmcrfileset` leaves it unlinked until `mmlinkfileset` |
| Write output | Success messages come from the GUI (`EFSSG0070I File set X created successfully.`) rather than the mm command's own wording |
| root quotas | The GUI rejects `--user root` / `--user 0` with *"The value 0 specified for u is invalid"*; GPFS does not enforce root quotas anyway |

### Quotas

Quota commands take the `Device[:Fileset]` operand of `mmsetquota` and
`mmlsquota`: `fs0` addresses the file system, `fs0:fset1` one fileset in it.

Which one to use is decided by the filesystem, not by preference. When
`mmlsfs <fs> --perfileset-quota` reports `yes` — the default for filesystems
serving CSI — user and group quotas exist per fileset only, and the GUI rejects
filesystem-scoped writes with *"Per fileset quota enabled on this filesystem"*.
Use `fs0:fset1` there. Reading with a bare `fs0` still works: mmctl walks the
filesets, as `mmlsquota` does, which costs one request per fileset.

A fileset's own quota is set through the fileset half of the operand with
neither `--user` nor `--group`: `mmctl mmsetquota fs0:fset1 --block 10G:20G`.

Limits use GPFS syntax (`10G`, `512M`, `1T`); `0` means no limit, and omitting
`HardLimit` leaves the hard limit unset. GPFS has no delete operation for a
quota — zero every limit to remove one.

Quota writes are asynchronous in the GUI: it answers `202` with a job handle.
mmctl follows the job to completion and reports its output, so a failure
surfaces as a non-zero exit rather than a silent accept.

Reads go through the GUI's own quota cache, which it refreshes a few seconds
after each write. An `mmlsquota` issued immediately after an `mmsetquota` can
therefore still show the previous limits; re-run it, or use `mmlsquota` on the
cluster for the authoritative value. This is GUI behaviour, not proxy caching —
mmapi forwards every request untouched.

```bash
# Per-fileset user and group quotas (per-fileset quota enabled)
mmctl mmsetquota fs0:fset1 --user ubuntu --block 1G:2G
mmctl mmsetquota fs0:fset1 --group ubuntu --block 1G:2G --files 1000:2000
mmctl mmlsquota -u ubuntu fs0:fset1
mmctl mmsetquota fs0:fset1 --user ubuntu --block 0:0 --files 0:0   # remove

# A fileset's own quota
mmctl mmsetquota fs0:fset1 --block 200G:200G --files 2000000:2048000
mmctl mmlsquota -j fset1 fs0

# Every quota in a fileset, or in the file system
mmctl mmrepquota fs0:fset1
mmctl mmrepquota -j fs0

# Filesystem-wide user quota (per-fileset quota disabled)
mmctl mmsetquota fs0 --user ubuntu --block 1G:2G
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
cp deploy/config-owning.json config.local.json && $EDITOR config.local.json
./deploy/deploy.sh root@<gpfs-node> ./mmapi ./config.local.json

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
