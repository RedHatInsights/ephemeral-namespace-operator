#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-res-default-pool"

# Verify reservation got assigned a namespace
NS=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.namespace}')
if [ -z "${NS}" ]; then
  echo "ERROR: Reservation has no namespace assigned"
  exit 1
fi
echo "PASS: Reservation assigned namespace: ${NS}"

# Verify the status pool was set to "default"
POOL=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.pool}')
if [ "${POOL}" != "default" ]; then
  echo "ERROR: Reservation pool is '${POOL}', expected 'default'"
  exit 1
fi
echo "PASS: Reservation defaulted to 'default' pool"

# Verify the namespace has the correct pool label
NS_POOL=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.labels.pool}')
if [ "${NS_POOL}" != "default" ]; then
  echo "ERROR: Namespace pool label is '${NS_POOL}', expected 'default'"
  exit 1
fi
echo "PASS: Namespace has pool=default label"

echo "SUCCESS: Reservation correctly defaulted to 'default' pool"
