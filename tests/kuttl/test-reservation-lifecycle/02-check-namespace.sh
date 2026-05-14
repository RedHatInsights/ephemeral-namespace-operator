#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-res-lifecycle"
POOL_NAME="test-res-lifecycle"

NS=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.namespace}')

if [ -z "${NS}" ]; then
  echo "ERROR: Reservation ${RESERVATION_NAME} has no namespace assigned"
  exit 1
fi

echo "Reservation assigned namespace: ${NS}"

# Save namespace name for cleanup verification in step 04
echo "${NS}" > /tmp/test-res-lifecycle-ns

# Verify reserved annotation
RESERVED=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.annotations.reserved}')
if [ "${RESERVED}" != "true" ]; then
  echo "ERROR: Namespace ${NS} is not marked as reserved (reserved=${RESERVED})"
  exit 1
fi
echo "PASS: Namespace ${NS} is marked as reserved"

# Verify pool label
POOL_LABEL=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.labels.pool}')
if [ "${POOL_LABEL}" != "${POOL_NAME}" ]; then
  echo "ERROR: Namespace pool label is '${POOL_LABEL}', expected '${POOL_NAME}'"
  exit 1
fi
echo "PASS: Namespace ${NS} has correct pool label"

# Verify owner reference points to NamespaceReservation (not NamespacePool)
OWNER_KIND=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.ownerReferences[0].kind}')
if [ "${OWNER_KIND}" != "NamespaceReservation" ]; then
  echo "ERROR: Namespace owner is '${OWNER_KIND}', expected 'NamespaceReservation'"
  exit 1
fi
echo "PASS: Namespace ${NS} is owned by NamespaceReservation"

# Verify pool status shows reserved: 1, ready: 0
POOL_READY=$(kubectl get namespacepool "${POOL_NAME}" -o jsonpath='{.status.ready}')
POOL_RESERVED=$(kubectl get namespacepool "${POOL_NAME}" -o jsonpath='{.status.reserved}')
if [ "${POOL_READY}" != "0" ]; then
  echo "ERROR: Pool ready count is ${POOL_READY}, expected 0"
  exit 1
fi
if [ "${POOL_RESERVED}" != "1" ]; then
  echo "ERROR: Pool reserved count is ${POOL_RESERVED}, expected 1"
  exit 1
fi
echo "PASS: Pool status shows ready=0, reserved=1"

echo "SUCCESS: All namespace checks passed"
