# mmapi

GPFS GUI REST API proxy for multi-tenant CSI support. Implements the IBM Storage Scale management API (`/scalemgmt/v2/`) so the GPFS CSI driver can connect without the full GUI service.

## Problem

When GPFS enables multi-filesystem or multi-fileset for multi-tenancy, the native GUI service runs with full cluster admin privileges. This makes it impossible to isolate tenants at the API level. mmapi solves this by:

1. Implementing the same REST API the CSI driver expects
2. Adding token-based access control per filesystem and fileset
3. Running as a lightweight systemd service on GPFS nodes

## Quick Start

```bash
# Build
make build-linux

# Deploy to a GPFS node
./deploy/deploy.sh root@10.243.145.103

# Create a token (restrict to fs0)
curl -X POST http://10.243.145.103:8080/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"allowedFs":["fs0"]}'

# Test with Scale API
curl -k -u admin:<token-secret> \
  http://10.243.145.103:8080/scalemgmt/v2/filesystems
```

## API Endpoints

### Scale GUI Compatible (for CSI driver)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/scalemgmt/v2/cluster` | Cluster info |
| GET | `/scalemgmt/v2/filesystems` | List filesystems |
| GET | `/scalemgmt/v2/filesystems/{fs}` | Get filesystem |
| POST | `/scalemgmt/v2/filesystems/{fs}/filesets` | Create fileset |
| GET | `/scalemgmt/v2/filesystems/{fs}/filesets/{fset}` | Get fileset |
| DELETE | `/scalemgmt/v2/filesystems/{fs}/filesets/{fset}` | Delete fileset |
| POST | `/scalemgmt/v2/filesystems/{fs}/filesets/{fset}/link` | Link fileset |
| DELETE | `/scalemgmt/v2/filesystems/{fs}/filesets/{fset}/link` | Unlink fileset |
| POST | `/scalemgmt/v2/filesystems/{fs}/quotas` | Set quota |
| GET | `/scalemgmt/v2/filesystems/{fs}/quotas` | Get quota |
| GET | `/scalemgmt/v2/jobs/{jobId}` | Job status |

### Token Management (admin)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/tokens` | Create token |
| GET | `/api/v1/tokens` | List tokens |
| DELETE | `/api/v1/tokens/{id}` | Delete token |

## Authentication

- **CSI driver**: Basic Auth, username=any, password=token secret
- **Token management**: No auth (bind to localhost or use firewall)

## Configuration

`/etc/mmapi/config.json`:

```json
{
  "port": 8080,
  "mode": "multi-fileset",
  "device": "fs0",
  "dataDir": "/var/lib/mmapi"
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `port` | HTTP listen port | 8080 |
| `mode` | `multi-fs` or `multi-fileset` | `multi-fileset` |
| `device` | Default GPFS device name | `gpfs0` |
| `dataDir` | Token storage directory | `/var/lib/mmapi` |

## Deployment

### Systemd

```bash
make build-linux
./deploy/deploy.sh root@<gpfs-node>
```

### GPFS CSI Integration

See [deploy/csi/](deploy/csi/) for Kubernetes deployment manifests.

In the CSI driver config, point the GUI host to mmapi:

```yaml
clusters:
  - id: owningcluster
    primary:
      primaryFS: fs0
    restApi:
      - guiHost: "10.243.145.103"
        guiPort: 8080
    secrets: mmapi-owning-secret
```

## Development

```bash
make test    # Run tests
make build   # Build for current platform
```
