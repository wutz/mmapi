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

### 1. Deploy mmapi (Systemd)

```bash
# Build linux binary
make build-linux

# Deploy to GPFS nodes
./deploy/deploy.sh root@<gpfs-node> ./mmapi ./deploy/config-owning.json

# Create access token
curl -X POST http://<gpfs-node>:8443/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"allowedFs":["fs0"]}'
# Save the returned secret for CSI configuration
```

### 2. Deploy GPFS CSI Driver

```bash
# Edit secrets with your token
vim deploy/csi/secrets.yaml

# Edit CSIScaleOperator CR with your cluster ID, filesystem, and mmapi host
vim deploy/csi/csiscaleoperator.yaml

# Run install script
./deploy/csi/install.sh

# Check status
kubectl -n ibm-spectrum-scale-csi-driver get pods
kubectl -n ibm-spectrum-scale-csi-driver get csiscaleoperator
```

### 3. Create StorageClass and Test

```bash
# Create StorageClass
kubectl apply -f deploy/csi/storageclass.yaml

# Deploy example pod with PVC
kubectl apply -f deploy/csi/example-pod.yaml

# Verify
kubectl get pvc test-gpfs-pvc
kubectl logs test-gpfs-pod
```

### CSI Manifests

| File | Description |
|------|-------------|
| `deploy/csi/install.sh` | Automated CSI operator install script |
| `deploy/csi/secrets.yaml` | Namespace and auth secret (edit token) |
| `deploy/csi/csiscaleoperator.yaml` | CSI operator CR (edit cluster ID, host, filesystem) |
| `deploy/csi/storageclass.yaml` | StorageClass for GPFS filesets |
| `deploy/csi/example-pod.yaml` | Example Pod + PVC for testing |

## Development

```bash
make test    # Run tests
make build   # Build for current platform
```
