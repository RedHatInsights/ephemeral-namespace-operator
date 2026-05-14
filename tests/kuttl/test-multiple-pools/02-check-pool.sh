#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-multi-pool-res"

NS=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.namespace}')

if [ -z "${NS}" ]; then
  echo "ERROR: Reservation ${RESERVATION_NAME} has no namespace assigned"
  exit 1
fi

echo "Reservation assigned namespace: ${NS}"

# Verify namespace belongs to pool-alpha (not pool-beta)
POOL_LABEL=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.labels.pool}')
if [ "${POOL_LABEL}" != "pool-alpha" ]; then
  echo "ERROR: Namespace pool label is '${POOL_LABEL}', expected 'pool-alpha'"
  exit 1
fi
echo "PASS: Namespace ${NS} belongs to pool-alpha"

# Verify reservation status shows correct pool
RES_POOL=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.pool}')
if [ "${RES_POOL}" != "pool-alpha" ]; then
  echo "ERROR: Reservation status pool is '${RES_POOL}', expected 'pool-alpha'"
  exit 1
fi
echo "PASS: Reservation status correctly shows pool-alpha"

echo "SUCCESS: Reservation correctly assigned to pool-alpha"
