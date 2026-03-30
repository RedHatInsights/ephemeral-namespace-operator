#!/bin/bash
set -euo pipefail

POOL_NAME="test-with-clowdenv"

# Find namespace(s) created by the pool
NS=$(kubectl get namespaces -l pool="${POOL_NAME}" -o jsonpath='{.items[0].metadata.name}')

if [ -z "${NS}" ]; then
  echo "ERROR: No namespace found for pool ${POOL_NAME}"
  exit 1
fi

echo "Found namespace: ${NS}"

# Verify the namespace has the expected labels and annotations
ENV_STATUS=$(kubectl get namespace "${NS}" -o jsonpath='{.metadata.annotations.env-status}')
echo "Namespace env-status: ${ENV_STATUS}"

if [ "${ENV_STATUS}" != "creating" ]; then
  echo "ERROR: Expected env-status 'creating' but got '${ENV_STATUS}'"
  exit 1
fi

# Verify ClowdEnvironment resource was created for this namespace
CLOWDENV_NAME="env-${NS}"

if ! kubectl get clowdenvironment "${CLOWDENV_NAME}" > /dev/null 2>&1; then
  echo "ERROR: ClowdEnvironment ${CLOWDENV_NAME} not found"
  echo "Listing all ClowdEnvironments:"
  kubectl get clowdenvironments || true
  exit 1
fi

echo "SUCCESS: ClowdEnvironment ${CLOWDENV_NAME} exists for namespace ${NS}"

# Verify the ClowdEnvironment targets the correct namespace
TARGET_NS=$(kubectl get clowdenvironment "${CLOWDENV_NAME}" -o jsonpath='{.spec.targetNamespace}')
if [ "${TARGET_NS}" != "${NS}" ]; then
  echo "ERROR: ClowdEnvironment targetNamespace is '${TARGET_NS}', expected '${NS}'"
  exit 1
fi

echo "SUCCESS: ClowdEnvironment targetNamespace correctly set to ${NS}"
