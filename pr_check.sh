
#!/bin/bash

set -exv

# Note, this does not currently work with podman. pr_check_inner.sh has insufficient permissions
RUNTIME="docker"
DOCKER_CONF="$PWD/.docker"
mkdir -p "$DOCKER_CONF"

# export IMAGE_TAG=`git rev-parse --short HEAD`
# export IMAGE_NAME=quay.io/cloudservices/ephemeral-namespace-operator

BASE_TAG=`cat go.mod go.sum Dockerfile | sha256sum  | head -c 8`
BASE_IMG=quay.io/cloudservices/ephemeral-namespace-operator:$BASE_TAG

CONTAINER_NAME="ephemeral-namespace-operator-pr-check-$ghprbPullId"
# NOTE: Make sure this volume is mounted 'ro', otherwise Jenkins cannot clean up the workspace due to file permission errors
set +e
# Run the pr check container (stored in the build dir) and invoke the
# pr_check_inner as its command
$RUNTIME run -i \
    --name $CONTAINER_NAME \
    -v $PWD:/workspace:ro \
    $IMAGE_NAME:IMAGE_TAG \
    make test

# TEST_RESULT=$?

# mkdir -p artifacts

# $RUNTIME cp $CONTAINER_NAME:/container_workspace/artifacts/ $PWD

# $RUNTIME rm -f $CONTAINER_NAME
# set -e

# exit $TEST_RESULT