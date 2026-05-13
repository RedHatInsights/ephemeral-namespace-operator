#!/bin/bash
set -euo pipefail

# Verify second reservation has no namespace assigned
NS=$(kubectl get namespacereservation test-exhaust-res-2 -o jsonpath='{.status.namespace}')

if [ -n "${NS}" ]; then
  echo "ERROR: Second reservation should not have a namespace, but got: ${NS}"
  exit 1
fi
echo "PASS: Second reservation has no namespace assigned"

# Verify first reservation still has its namespace
NS1=$(kubectl get namespacereservation test-exhaust-res-1 -o jsonpath='{.status.namespace}')
if [ -z "${NS1}" ]; then
  echo "ERROR: First reservation should still have a namespace"
  exit 1
fi
echo "PASS: First reservation still holds namespace ${NS1}"

# Verify pool status
POOL_READY=$(kubectl get namespacepool test-exhausted-pool -o jsonpath='{.status.ready}')
POOL_RESERVED=$(kubectl get namespacepool test-exhausted-pool -o jsonpath='{.status.reserved}')
if [ "${POOL_READY}" != "0" ]; then
  echo "ERROR: Pool ready is ${POOL_READY}, expected 0"
  exit 1
fi
if [ "${POOL_RESERVED}" != "1" ]; then
  echo "ERROR: Pool reserved is ${POOL_RESERVED}, expected 1"
  exit 1
fi
echo "PASS: Pool correctly shows ready=0, reserved=1 (exhausted)"

echo "SUCCESS: Exhausted pool correctly blocks second reservation"
