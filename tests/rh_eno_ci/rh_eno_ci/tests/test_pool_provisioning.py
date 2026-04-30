"""Tests for NamespacePool provisioning with a ClowdEnvironment."""

import logging

import pytest

from .conftest import (
    cleanup_pool,
    create_pool,
    oc_json,
    wait_for_pool_namespace,
)

logger = logging.getLogger(__name__)

POOL_NAME = "e2e-test-with-clowdenv"

CLOWDENV_SPEC = {
    "resourceDefaults": {
        "limits": {"cpu": "300m", "memory": "256Mi"},
        "requests": {"cpu": "30m", "memory": "128Mi"},
    },
    "providers": {
        "kafka": {"mode": "none"},
        "db": {"mode": "none"},
        "inMemoryDb": {"mode": "none"},
        "logging": {"mode": "none"},
        "metrics": {"mode": "none", "port": 9000},
        "objectStore": {"mode": "none"},
        "web": {"mode": "none", "port": 8000},
        "featureFlags": {"mode": "none"},
    },
}


@pytest.fixture(scope="module")
def pool_namespace():
    """Create a pool with ClowdEnvironment and return the first namespace name."""
    create_pool(POOL_NAME, size=1, clowdenv_spec=CLOWDENV_SPEC)
    ns_name = wait_for_pool_namespace(POOL_NAME)
    yield ns_name
    cleanup_pool(POOL_NAME)


class TestPoolProvisioning:
    def test_namespace_has_pool_label(self, pool_namespace):
        ns = oc_json("get", "namespace", pool_namespace)
        assert ns["metadata"]["labels"]["pool"] == POOL_NAME

    def test_namespace_has_operator_label(self, pool_namespace):
        ns = oc_json("get", "namespace", pool_namespace)
        assert ns["metadata"]["labels"]["operator-ns"] == "true"

    def test_namespace_annotated_not_reserved(self, pool_namespace):
        ns = oc_json("get", "namespace", pool_namespace)
        assert ns["metadata"]["annotations"]["reserved"] == "false"

    def test_clowdenv_exists_with_correct_target(self, pool_namespace):
        env_name = f"env-{pool_namespace}"
        env = oc_json("get", "clowdenvironment", env_name, "-n", pool_namespace)
        assert env["spec"]["targetNamespace"] == pool_namespace

    def test_clowdenv_providers_match_spec(self, pool_namespace):
        env_name = f"env-{pool_namespace}"
        env = oc_json("get", "clowdenvironment", env_name, "-n", pool_namespace)
        providers = env["spec"]["providers"]
        assert providers["kafka"]["mode"] == "none"
        assert providers["db"]["mode"] == "none"
        assert providers["logging"]["mode"] == "none"

    def test_limitrange_created(self, pool_namespace):
        lr = oc_json("get", "limitrange", "resource-limits", "-n", pool_namespace)
        assert lr["metadata"]["name"] == "resource-limits"

    def test_resourcequota_created(self, pool_namespace):
        rq = oc_json("get", "resourcequota", "compute-resources", "-n", pool_namespace)
        assert rq["metadata"]["name"] == "compute-resources"

    def test_namespace_owned_by_pool(self, pool_namespace):
        ns = oc_json("get", "namespace", pool_namespace)
        owner_refs = ns["metadata"].get("ownerReferences", [])
        assert len(owner_refs) > 0
        assert owner_refs[0]["kind"] == "NamespacePool"
        assert owner_refs[0]["name"] == POOL_NAME
