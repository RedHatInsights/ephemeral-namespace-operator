#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-reservation-secret-source"

# Get the namespace assigned to the reservation
NS=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.namespace}')

if [ -z "${NS}" ]; then
  echo "ERROR: Reservation ${RESERVATION_NAME} has no namespace assigned"
  exit 1
fi

echo "Reserved namespace: ${NS}"

# Verify secrets with correct qontract annotation WERE copied

echo "--- Checking vault-managed-secret ---"
if ! kubectl get secret vault-managed-secret -n "${NS}" > /dev/null 2>&1; then
  echo "ERROR: vault-managed-secret was not copied to namespace ${NS}"
  echo "Secrets in namespace:"
  kubectl get secrets -n "${NS}" --no-headers
  exit 1
fi
echo "OK: vault-managed-secret found"

# Verify the secret data was preserved
API_KEY=$(kubectl get secret vault-managed-secret -n "${NS}" -o jsonpath='{.data.api-key}')
if [ "${API_KEY}" != "dGVzdC1hcGkta2V5" ]; then
  echo "ERROR: vault-managed-secret data mismatch: expected 'dGVzdC1hcGkta2V5', got '${API_KEY}'"
  exit 1
fi
echo "OK: vault-managed-secret data preserved"

echo "--- Checking rhcs-cert-secret ---"
if ! kubectl get secret rhcs-cert-secret -n "${NS}" > /dev/null 2>&1; then
  echo "ERROR: rhcs-cert-secret was not copied to namespace ${NS}"
  exit 1
fi
echo "OK: rhcs-cert-secret found"

# Verify secrets that should NOT have been copied

echo "--- Checking unmanaged-secret is absent ---"
if kubectl get secret unmanaged-secret -n "${NS}" > /dev/null 2>&1; then
  echo "ERROR: unmanaged-secret should NOT have been copied (missing qontract annotation)"
  exit 1
fi
echo "OK: unmanaged-secret correctly not copied"

echo "--- Checking ignored-secret is absent ---"
if kubectl get secret ignored-secret -n "${NS}" > /dev/null 2>&1; then
  echo "ERROR: ignored-secret should NOT have been copied (bonfire.ignore=true)"
  exit 1
fi
echo "OK: ignored-secret correctly not copied"

echo "SUCCESS: All secret copy assertions passed for namespace ${NS}"
