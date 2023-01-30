#!/bin/bash

go version

echo "$MINIKUBE_SSH_KEY" > minikube-ssh-ident

while read line; do
    if [ ${#line} -ge 100 ]; then
        echo "Commit messages are limited to 100 characters."
        echo "The following commit message has ${#line} characters."
        echo "${line}"
        exit 1
    fi
done <<< "$(git log --pretty=format:%s $(git merge-base main HEAD)..HEAD)"

set -exv

BASE_TAG=`cat go.mod go.sum Dockerfile | sha256sum  | head -c 8`
BASE_IMG=quay.io/cloudservices/ephemeral-namespace-operator:$BASE_TAG

DOCKER_CONF="$PWD/.docker"
mkdir -p "$DOCKER_CONF"
docker login -u="$QUAY_USER" -p="$QUAY_TOKEN" quay.io

RESPONSE=$( \
        curl -Ls -H "Authorization: Bearer $QUAY_API_TOKEN" \
        "https://quay.io/api/v1/repository/cloudservices/ephemeral-namespace-operator/tag/?specificTag=$BASE_TAG" \
    )

echo "received HTTP response: $RESPONSE"

# find all non-expired tags
VALID_TAGS_LENGTH=$(echo $RESPONSE | jq '[ .tags[] | select(.end_ts == null) ] | length')

if [[ "$VALID_TAGS_LENGTH" -eq 0 ]]; then
    BASE_IMG=$BASE_IMG make docker-build-and-push-base
fi

export IMAGE_TAG=`git rev-parse --short=8 HEAD`
export IMAGE_NAME=quay.io/cloudservices/ephemeral-namespace-operator

echo $BASE_IMG

make update-version

TEST_CONT="ephemeral-namespace-operator-unit-"$IMAGE_TAG
docker build -t $TEST_CONT -f Dockerfile.test --build-arg BASE_IMAGE=$BASE_IMG . 

echo $(pwd)
echo "--------------------------------------------------------"
echo $(ls -l)
echo "--------------------------------------------------------"
echo $(ls -l bin)
echo "--------------------------------------------------------"
echo $(which setup-envtest)

GO_TOOLSET_IMAGE='registry.access.redhat.com/ubi9/go-toolset:1.18.9'

docker run -i \
    -v "$(pwd):/bins:ro" \
    --workdir /workdir --name "$TEST_CONT" \
    "$GO_TOOLSET_IMAGE" make test
UNIT_TEST_RESULT=$?

if [[ $UNIT_TEST_RESULT -ne 0 ]]; then
    exit $UNIT_TEST_RESULT
fi

ENO_VERSION=`git describe --tags`

IMG=$IMAGE_NAME:$IMAGE_TAG BASE_IMG=$BASE_IMG make docker-build
IMG=$IMAGE_NAME:$IMAGE_TAG make docker-push

docker rm enocopy || true
docker create --name enocopy $IMAGE_NAME:$IMAGE_TAG
docker cp enocopy:/manifest.yaml .
docker rm enocopy || true

CONTAINER_NAME="ephemeral-namespace-operator-pr-check-$ghprbPullId"
docker rm -f $CONTAINER_NAME || true
# NOTE: Make sure this volume is mounted 'ro', otherwise Jenkins cannot clean up the workspace due to file permission errors
set +e
docker run -i \
    --name $CONTAINER_NAME \
    -v $PWD:/workspace:ro \
    -v `$PWD/bin/setup-envtest use -p path`:/bins:ro \
    -e IMAGE_NAME=$IMAGE_NAME \
    -e IMAGE_TAG=$IMAGE_TAG \
    -e QUAY_USER=$QUAY_USER \
    -e QUAY_TOKEN=$QUAY_TOKEN \
    -e MINIKUBE_HOST=$MINIKUBE_HOST \
    -e MINIKUBE_ROOTDIR=$MINIKUBE_ROOTDIR \
    -e MINIKUBE_USER=$MINIKUBE_USER \
    -e ENO_VERSION=$ENO_VERSION \
    $BASE_IMG \
    /workspace/build/pr_check_inner.sh
TEST_RESULT=$?

mkdir artifacts

docker cp $CONTAINER_NAME:/container_workspace/artifacts/ $PWD

docker rm -f $CONTAINER_NAME
set -e

exit $TEST_RESULT
