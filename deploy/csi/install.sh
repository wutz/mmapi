#!/bin/bash
set -euo pipefail

# Deploy IBM Spectrum Scale CSI driver with mmapi
# Prerequisites:
#   - mmapi running on GPFS nodes with HTTPS and tokens created
#   - kubectl configured with target cluster

NAMESPACE="ibm-spectrum-scale-csi-driver"
OPERATOR_IMAGE="quay.io/ibm-spectrum-scale/ibm-spectrum-scale-csi-operator:v2.12.0"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Step 1: Install CRD ==="
kubectl apply -f https://raw.githubusercontent.com/IBM/ibm-spectrum-scale-csi/refs/heads/master/operator/config/crd/bases/csi.ibm.com_csiscaleoperators.yaml

echo "=== Step 2: Create namespace ==="
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "=== Step 3: Install RBAC ==="
kubectl apply -f https://raw.githubusercontent.com/IBM/ibm-spectrum-scale-csi/refs/heads/master/operator/config/rbac/role.yaml
kubectl apply -n "$NAMESPACE" -f https://raw.githubusercontent.com/IBM/ibm-spectrum-scale-csi/refs/heads/master/operator/config/rbac/service_account.yaml
kubectl apply -f https://raw.githubusercontent.com/IBM/ibm-spectrum-scale-csi/refs/heads/master/operator/config/rbac/role_binding.yaml

echo "=== Step 4: Deploy operator ==="
kubectl apply -n "$NAMESPACE" -f https://raw.githubusercontent.com/IBM/ibm-spectrum-scale-csi/refs/heads/master/operator/config/manager/manager.yaml

echo "=== Step 5: Fix operator image (use quay.io) ==="
kubectl -n "$NAMESPACE" set image deployment/ibm-spectrum-scale-csi-operator '*'="$OPERATOR_IMAGE"

echo "=== Step 6: Wait for operator ==="
kubectl -n "$NAMESPACE" rollout status deployment/ibm-spectrum-scale-csi-operator --timeout=120s

echo "=== Step 7: Apply secrets ==="
kubectl apply -f "$SCRIPT_DIR/secrets.yaml"

echo "=== Step 8: Apply CSIScaleOperator CR ==="
kubectl apply -f "$SCRIPT_DIR/csiscaleoperator.yaml"

echo ""
echo "CSI deployment initiated. Check status:"
echo "  kubectl -n $NAMESPACE get pods"
echo "  kubectl -n $NAMESPACE get csiscaleoperator"
