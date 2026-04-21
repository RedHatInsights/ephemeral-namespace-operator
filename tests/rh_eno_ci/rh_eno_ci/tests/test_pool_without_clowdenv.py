"""Tests for NamespacePool provisioning without a ClowdEnvironment."""

import logging

import pytest

from .conftest import (
    cleanup_pool,
    create_pool,
    oc_json,
    wait_for_pool_ready,
    wait_for_ready_pool_namespace,
)

logger = logging.getLogger(__name__)

POOL_NAME = "e2e-test-without-clowdenv"


@pytest.fixture(scope="module")
def pool_namespace():
    """Create a pool without ClowdEnvironment and return the first namespace name."""
    create_pool(POOL_NAME, size=2)
    ns_name = wait_for_ready_pool_namespace(POOL_NAME)
    yield ns_name
    cleanup_pool(POOL_NAME)


class TestPoolWithoutClowdEnv:
    def test_namespace_ready_immediately(self, pool_namespace):
        assert pool_namespace.startswith("ephemeral-")

    def test_no_clowdenvironment_created(self, pool_namespace):
        env_name = f"env-{pool_namespace}"
        try:
            oc_json("get", "clowdenvironment", env_name, "-n", pool_namespace)
            pytest.fail(f"ClowdEnvironment {env_name} should not exist")
        except Exception:
            pass

    def test_limitrange_still_created(self, pool_namespace):
        lr = oc_json("get", "limitrange", "resource-limits", "-n", pool_namespace)
        assert lr["metadata"]["name"] == "resource-limits"

    def test_resourcequota_still_created(self, pool_namespace):
        rq = oc_json("get", "resourcequota", "compute-resources", "-n", pool_namespace)
        assert rq["metadata"]["name"] == "compute-resources"

    def test_pool_status_ready_count(self, pool_namespace):
        wait_for_pool_ready(POOL_NAME, expected_ready=2)
        pool = oc_json("get", "namespacepool", POOL_NAME)
        assert pool["status"]["ready"] == 2
        assert pool["status"]["creating"] == 0
