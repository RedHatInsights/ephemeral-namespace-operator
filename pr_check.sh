#!/bin/bash

set -x

CONTAINER_ENGINE_CMD=''
TEST_CONTAINER_NAME=''
TEARDOWN_RAN=0
GO_TOOLSET_IMAGE='registry.access.redhat.com/ubi8/go-toolset:1.19.9-2.1687187497'

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

    trap teardown EXIT ERR SIGINT SIGTERM

    mkdir -p artifacts

    TEST_CONTAINER_NAME="ENO-$(get_N_chars_commit_hash 7)"

    container_engine_cmd run -d --name "$TEST_CONTAINER_NAME" \
        "$GO_TOOLSET_IMAGE" sleep infinity

    container_engine_cmd cp -a . "${TEST_CONTAINER_NAME}:/workdir"


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

main
