#!/bin/bash
set -euo pipefail

RESERVATION_NAME="test-res-lifecycle"

# Read namespace name saved by step 02
NS_FILE="/tmp/test-res-lifecycle-ns"
if [ ! -f "${NS_FILE}" ]; then
  echo "ERROR: Namespace name file not found at ${NS_FILE}"
  exit 1
fi
NS=$(cat "${NS_FILE}")
rm -f "${NS_FILE}"

echo "Waiting for namespace ${NS} to be deleted after reservation cleanup..."

# Wait up to 120 seconds for namespace to be deleted
for i in $(seq 1 24); do
  if ! kubectl get namespace "${NS}" > /dev/null 2>&1; then
    echo "PASS: Namespace ${NS} has been deleted"
    break
  fi

  STATUS=$(kubectl get namespace "${NS}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "gone")
  echo "  Attempt ${i}/24: Namespace ${NS} still exists (phase: ${STATUS})"
  sleep 5
done

# Final check
if kubectl get namespace "${NS}" > /dev/null 2>&1; then
  STATUS=$(kubectl get namespace "${NS}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "unknown")
  echo "ERROR: Namespace ${NS} still exists after 120s (phase: ${STATUS})"
  exit 1
fi

# Verify reservation is also gone
if kubectl get namespacereservation "${RESERVATION_NAME}" > /dev/null 2>&1; then
  echo "ERROR: Reservation ${RESERVATION_NAME} still exists"
  exit 1
fi

echo "PASS: Reservation and namespace have been cleaned up"
