#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-reservation-no-clowdenv"
POOL_NAME="test-without-clowdenv"

# Get the namespace assigned to the reservation
NS=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.namespace}')

if [ -z "${NS}" ]; then
  echo "ERROR: Reservation ${RESERVATION_NAME} has no namespace assigned"
  exit 1
fi

echo "Reservation assigned namespace: ${NS}"

# Verify the namespace exists and is marked as reserved
RESERVED=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.annotations.reserved}')
if [ "${RESERVED}" != "true" ]; then
  echo "ERROR: Namespace ${NS} is not marked as reserved (reserved=${RESERVED})"
  exit 1
fi

echo "Namespace ${NS} is correctly marked as reserved"

# Verify the namespace belongs to the expected pool
POOL_LABEL=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.labels.pool}')
if [ "${POOL_LABEL}" != "${POOL_NAME}" ]; then
  echo "ERROR: Namespace pool label is '${POOL_LABEL}', expected '${POOL_NAME}'"
  exit 1
fi

echo "Namespace ${NS} belongs to pool ${POOL_NAME}"

# Verify NO ClowdEnvironment exists for this namespace
CLOWDENV_NAME="env-${NS}"

if kubectl get clowdenvironment "${CLOWDENV_NAME}" > /dev/null 2>&1; then
  echo "ERROR: ClowdEnvironment ${CLOWDENV_NAME} should NOT exist for a pool without ClowdEnvironment"
  exit 1
fi

echo "SUCCESS: No ClowdEnvironment found for namespace ${NS} (as expected)"
