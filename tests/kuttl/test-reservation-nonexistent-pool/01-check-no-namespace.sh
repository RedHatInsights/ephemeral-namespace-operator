#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-res-no-pool"

# Verify reservation has no namespace assigned
NS=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.namespace}')

if [ -n "${NS}" ]; then
  echo "ERROR: Reservation should not have a namespace assigned, but got: ${NS}"
  exit 1
fi
echo "PASS: Reservation has no namespace assigned"

# Verify reservation is still in waiting state
STATE=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.state}')
if [ "${STATE}" != "waiting" ]; then
  echo "ERROR: Reservation state is '${STATE}', expected 'waiting'"
  exit 1
fi
echo "PASS: Reservation is in waiting state"

# Verify the pool field in status shows the requested pool
POOL=$(kubectl get namespacereservation "${RESERVATION_NAME}" -o jsonpath='{.status.pool}')
if [ "${POOL}" != "this-pool-does-not-exist" ]; then
  echo "ERROR: Reservation pool is '${POOL}', expected 'this-pool-does-not-exist'"
  exit 1
fi
echo "PASS: Reservation correctly references non-existent pool"

echo "SUCCESS: Reservation correctly waiting for non-existent pool"
