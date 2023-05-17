#!/bin/bash

TEST_RESULT=0

mkdir -p artifacts

TEST_CONTAINER_NAME="ENO-$(get_N_chars_commit_hash 7)"

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
