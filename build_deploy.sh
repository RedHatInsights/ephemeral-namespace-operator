#!/bin/bash

set -exv

IMAGE="quay.io/cloudservices/ephemeral-namespace-operator"
IMAGE_TAG=$(git rev-parse --short=7 HEAD)

if [[ -z "$QUAY_USER" || -z "$QUAY_TOKEN" ]]; then
    echo "QUAY_USER and QUAY_TOKEN must be set"
    exit 1
fi

if [[ -z "$RH_REGISTRY_USER" || -z "$RH_REGISTRY_TOKEN" ]]; then
    echo "RH_REGISTRY_USER and RH_REGISTRY_TOKEN  must be set"
    exit 1
fi

docker login -u="$QUAY_USER" -p="$QUAY_TOKEN" quay.io
docker login -u="$RH_REGISTRY_USER" -p="$RH_REGISTRY_TOKEN" registry.redhat.io

make update-version

# Check if the multiarchbuilder exists
if docker buildx ls | grep -q "multiarchbuilder"; then
    echo "Using multiarchbuilder for buildx"
    # Multi-architecture build
    docker buildx use multiarchbuilder
    docker buildx build --platform linux/amd64,linux/arm64 -t "${IMAGE}:${IMAGE_TAG}" --push .
else
    echo "Falling back to standard build and push"
    # Standard build and push
    docker build -t "${IMAGE}:${IMAGE_TAG}" .
    docker push "${IMAGE}:${IMAGE_TAG}"
fi


