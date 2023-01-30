#!/bin/bash

set -x

env

WORKDIR=$(pwd)
CONTAINER_ENGINE_CMD=''
TEST_CONTAINER_NAME=''
TEARDOWN_RAN=0
GO_TOOLSET_IMAGE='registry.access.redhat.com/ubi9/go-toolset:1.18.9'

get_N_chars_commit_hash() {

    local CHARS=${1:-7}
    git rev-parse --short="$CHARS" HEAD
}

set_container_engine_cmd() {

    if container_engine_available 'podman'; then
        CONTAINER_ENGINE_CMD='podman'
    elif container_engine_available 'docker'; then
        CONTAINER_ENGINE_CMD='docker'
    else
        echo "ERROR, no container engine found, please install either podman or docker first"
        return 1
    fi

    echo "Container engine selected: $CONTAINER_ENGINE_CMD"
}

container_engine_available() {

    local CONTAINER_ENGINE_CMD="$1"
    local CONTAINER_ENGINE_AVAILABLE=1

    if [ "$CONTAINER_ENGINE_CMD" = "podman" ]; then
        if _command_is_present 'podman'; then
            CONTAINER_ENGINE_AVAILABLE=0
        fi
    elif [ "$CONTAINER_ENGINE_CMD" = "docker" ]; then
        if _command_is_present 'docker' && ! _docker_seems_emulated; then
            CONTAINER_ENGINE_AVAILABLE=0
        fi
    fi

    return "$CONTAINER_ENGINE_AVAILABLE"
}

_command_is_present() {
    command -v "$1" > /dev/null 2>&1
}

_docker_seems_emulated() {

    local DOCKER_COMMAND_PATH
    DOCKER_COMMAND_PATH=$(command -v docker)

    if [[ $(file "$DOCKER_COMMAND_PATH") == *"ASCII text"* ]]; then
        return 0
    fi
    return 1
}

init() {
    set_container_engine_cmd
}

teardown() {

    [ "$TEARDOWN_RAN" -ne "0" ] && return

    echo "Running teardown..."

    container_engine_cmd rm -f "$TEST_CONTAINER_NAME"
    TEARDOWN_RAN=1
}

container_engine_cmd() {

    if [ -z "$CONTAINER_ENGINE_CMD" ]; then
        if ! set_container_engine_cmd; then
            return 1
        fi
    fi

    if [ "$CONTAINER_ENGINE_CMD" = "podman" ]; then
        podman "$@"
    else
        docker "$@"
    fi
}

main() {

    local TEST_RESULT=0

    mkdir -p artifacts

    TEST_CONTAINER_NAME="ENO-$(get_N_chars_commit_hash)"

    container_engine_cmd run -d --name "$TEST_CONTAINER_NAME" \
        "$GO_TOOLSET_IMAGE" sleep infinity


    container_engine_cmd cp -a . "$TEST_CONTAINER_NAME:/workdir"


    container_engine_cmd exec --workdir /workdir "$TEST_CONTAINER_NAME" make test > 'artifacts/test_logs.txt'
    TEST_RESULT=$?

    container_engine_cmd cp "$TEST_CONTAINER_NAME:/workdir/cover.out" 'artifacts/cover.out'
    container_engine_cmd cp "$TEST_CONTAINER_NAME:/workdir/junit-eno.xml" 'artifacts/junit-eno.xml'

    if [ $TEST_RESULT -eq 0 ]; then
        echo "tests ran successfully"
    else
        echo "tests failed"
        return $TEST_RESULT
    fi
}

trap teardown EXIT ERR SIGINT SIGTERM

main

#make test
#
#env
#exit 1

#go version

#echo "$MINIKUBE_SSH_KEY" > minikube-ssh-ident
#
#while read line; do
#    if [ ${#line} -ge 100 ]; then
#        echo "Commit messages are limited to 100 characters."
#        echo "The following commit message has ${#line} characters."
#        echo "${line}"
#        exit 1
#    fi
#done <<< "$(git log --pretty=format:%s $(git merge-base main HEAD)..HEAD)"
#
#set -exv

#BASE_TAG=`cat go.mod go.sum Dockerfile | sha256sum  | head -c 8`
#BASE_IMG=quay.io/cloudservices/ephemeral-namespace-operator:$BASE_TAG

#DOCKER_CONF="$PWD/.docker"
#mkdir -p "$DOCKER_CONF"
#docker login -u="$QUAY_USER" -p="$QUAY_TOKEN" quay.io

#RESPONSE=$( \
#        curl -Ls -H "Authorization: Bearer $QUAY_API_TOKEN" \
#        "https://quay.io/api/v1/repository/cloudservices/ephemeral-namespace-operator/tag/?specificTag=$BASE_TAG" \
#    )
#
#echo "received HTTP response: $RESPONSE"

# find all non-expired tags
#VALID_TAGS_LENGTH=$(echo $RESPONSE | jq '[ .tags[] | select(.end_ts == null) ] | length')

#if [[ "$VALID_TAGS_LENGTH" -eq 0 ]]; then
#    BASE_IMG=$BASE_IMG make docker-build-and-push-base
#fi

#export IMAGE_TAG=`git rev-parse --short=8 HEAD`
#export IMAGE_NAME=quay.io/cloudservices/ephemeral-namespace-operator

#echo $BASE_IMG

#make update-version

#TEST_CONT="ephemeral-namespace-operator-unit-"$IMAGE_TAG
#docker build -t $TEST_CONT -f Dockerfile.test --build-arg BASE_IMAGE=$BASE_IMG . 

#docker run -i \
#    -v `$PWD/testbin/setup-envtest.sh use -p path`:/bins:ro \
#    -e IMAGE_NAME=$IMAGE_NAME \
#    -e IMAGE_TAG=$IMAGE_TAG \
#    -e QUAY_USER=$QUAY_USER \
#    -e QUAY_TOKEN=$QUAY_TOKEN \
#    -e MINIKUBE_HOST=$MINIKUBE_HOST \
#    -e MINIKUBE_ROOTDIR=$MINIKUBE_ROOTDIR \
#    -e MINIKUBE_USER=$MINIKUBE_USER \
#    -e ENO_VERSION=$ENO_VERSION \
#    $TEST_CONT \
#    make test
#UNIT_TEST_RESULT=$?
#
#if [[ $UNIT_TEST_RESULT -ne 0 ]]; then
#    exit $UNIT_TEST_RESULT
#fi
#
#ENO_VERSION=`git describe --tags`
#
#IMG=$IMAGE_NAME:$IMAGE_TAG BASE_IMG=$BASE_IMG make docker-build
#IMG=$IMAGE_NAME:$IMAGE_TAG make docker-push
#
#docker rm enocopy || true
#docker create --name enocopy $IMAGE_NAME:$IMAGE_TAG
#docker cp enocopy:/manifest.yaml .
#docker rm enocopy || true
#
#CONTAINER_NAME="ephemeral-namespace-operator-pr-check-$ghprbPullId"
#docker rm -f $CONTAINER_NAME || true
## NOTE: Make sure this volume is mounted 'ro', otherwise Jenkins cannot clean up the workspace due to file permission errors
#set +e
#docker run -i \
#    --name $CONTAINER_NAME \
#    -v $PWD:/workspace:ro \
#    -v `$PWD/bin/setup-envtest use -p path`:/bins:ro \
#    -e IMAGE_NAME=$IMAGE_NAME \
#    -e IMAGE_TAG=$IMAGE_TAG \
#    -e QUAY_USER=$QUAY_USER \
#    -e QUAY_TOKEN=$QUAY_TOKEN \
#    -e MINIKUBE_HOST=$MINIKUBE_HOST \
#    -e MINIKUBE_ROOTDIR=$MINIKUBE_ROOTDIR \
#    -e MINIKUBE_USER=$MINIKUBE_USER \
#    -e ENO_VERSION=$ENO_VERSION \
#    $BASE_IMG \
#    /workspace/build/pr_check_inner.sh
#TEST_RESULT=$?
#
#mkdir artifacts
#
#docker cp $CONTAINER_NAME:/container_workspace/artifacts/ $PWD
#
#docker rm -f $CONTAINER_NAME
#set -e

#exit $TEST_RESULT
