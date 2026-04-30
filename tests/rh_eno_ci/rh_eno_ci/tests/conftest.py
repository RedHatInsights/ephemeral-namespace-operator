import json
import logging
import os

import pytest
from ocviapy import oc
from wait_for import wait_for

logger = logging.getLogger(__name__)

DEFAULT_NAMESPACE = os.environ.get("TEST_NS", "ephemeral-namespace-operator-system")
E2E_PREFIX = "e2e-test-"


def oc_json(*args, **kwargs):
    """Run an oc command and return parsed JSON output."""
    result = str(oc(*args, "-o", "json", _silent=True, **kwargs))
    return json.loads(result)


def create_pool(name, size=2, clowdenv_spec=None, secret_source_ns=None):
    """Create a NamespacePool and return its name."""
    pool = {
        "apiVersion": "cloud.redhat.com/v1alpha1",
        "kind": "NamespacePool",
        "metadata": {"name": name},
        "spec": {
            "size": size,
            "local": True,
            "limitrange": {
                "metadata": {"name": "resource-limits"},
                "spec": {
                    "limits": [{
                        "type": "Container",
                        "default": {"cpu": "200m", "memory": "512Mi"},
                        "defaultRequest": {"cpu": "100m", "memory": "384Mi"},
                    }]
                },
            },
            "resourcequotas": {
                "items": [{
                    "metadata": {"name": "compute-resources"},
                    "spec": {"hard": {"pods": "10"}},
                }]
            },
        },
    }
    if clowdenv_spec:
        pool["spec"]["clowdenvironment"] = clowdenv_spec
    if secret_source_ns:
        pool["spec"]["defaultSecretSourceNamespace"] = secret_source_ns

    oc("apply", "-f", "-", _in=json.dumps(pool))
    logger.info("Created NamespacePool %s (size=%d)", name, size)
    return name


def create_reservation(name, pool, duration="30m", requester="e2e-test"):
    """Create a NamespaceReservation and return its name."""
    res = {
        "apiVersion": "cloud.redhat.com/v1alpha1",
        "kind": "NamespaceReservation",
        "metadata": {"name": name},
        "spec": {
            "requester": requester,
            "duration": duration,
            "pool": pool,
        },
    }
    oc("apply", "-f", "-", _in=json.dumps(res))
    logger.info("Created NamespaceReservation %s (pool=%s, duration=%s)", name, pool, duration)
    return name


def wait_for_pool_namespace(pool_name, timeout=240):
    """Wait for at least one Active namespace with label pool=<name>. Return its name."""
    def _check():
        try:
            result = oc_json("get", "namespaces", "-l", f"pool={pool_name}")
            for ns in result.get("items", []):
                phase = ns.get("status", {}).get("phase")
                env_status = ns.get("metadata", {}).get("annotations", {}).get("env-status", "")
                if phase == "Active" and env_status != "deleting":
                    return ns["metadata"]["name"]
        except Exception:
            pass
        return None

    ns_name, _ = wait_for(_check, timeout=timeout, delay=5, fail_condition=None,
                          message=f"waiting for namespace in pool {pool_name}")
    assert ns_name, f"timed out waiting for namespace in pool {pool_name}"
    return ns_name


def wait_for_ready_pool_namespace(pool_name, timeout=240):
    """Wait for a ready namespace in the pool. Returns its name.

    Combines namespace discovery and readiness into a single poll to avoid
    races where the operator deletes and recreates namespaces.
    """
    def _check():
        try:
            result = oc_json("get", "namespaces", "-l", f"pool={pool_name}")
            for ns in result.get("items", []):
                phase = ns.get("status", {}).get("phase")
                env_status = ns.get("metadata", {}).get("annotations", {}).get("env-status", "")
                if phase == "Active" and env_status == "ready":
                    return ns["metadata"]["name"]
        except Exception:
            pass
        return None

    ns_name, _ = wait_for(_check, timeout=timeout, delay=5, fail_condition=None,
                          message=f"waiting for ready namespace in pool {pool_name}")
    assert ns_name, f"timed out waiting for ready namespace in pool {pool_name}"
    return ns_name


def wait_for_namespace_ready(ns_name, timeout=240):
    """Wait for a namespace to have env-status=ready annotation."""
    def _check():
        try:
            ns = oc_json("get", "namespace", ns_name)
            annotations = ns.get("metadata", {}).get("annotations", {})
            return annotations.get("env-status") == "ready"
        except Exception:
            return False

    result, _ = wait_for(_check, timeout=timeout, delay=5,
                         message=f"waiting for namespace {ns_name} to become ready")
    assert result, f"timed out waiting for namespace {ns_name} to become ready"


def wait_for_reservation_active(res_name, timeout=300):
    """Wait for a reservation to reach active state. Return the assigned namespace."""
    def _check():
        try:
            res = oc_json("get", "namespacereservation", res_name)
            status = res.get("status", {})
            if status.get("state") == "active":
                return status.get("namespace")
        except Exception:
            pass
        return None

    ns_name, _ = wait_for(_check, timeout=timeout, delay=5, fail_condition=None,
                          message=f"waiting for reservation {res_name} to become active")
    assert ns_name, f"timed out waiting for reservation {res_name} to become active"
    return ns_name


def wait_for_pool_ready(pool_name, expected_ready, timeout=240):
    """Wait for a pool's status.ready to reach expected count."""
    def _check():
        try:
            pool = oc_json("get", "namespacepool", pool_name)
            return pool.get("status", {}).get("ready", 0) == expected_ready
        except Exception:
            return False

    result, _ = wait_for(_check, timeout=timeout, delay=5,
                         message=f"waiting for pool {pool_name} to have {expected_ready} ready")
    assert result, f"timed out waiting for pool {pool_name} to have {expected_ready} ready"


def cleanup_pool(pool_name):
    """Delete a pool and all namespaces labeled with that pool name."""
    logger.info("Cleaning up pool %s", pool_name)
    try:
        result = oc_json("get", "namespaces", "-l", f"pool={pool_name}")
        for ns in result.get("items", []):
            ns_name = ns["metadata"]["name"]
            oc("delete", "namespace", ns_name, "--ignore-not-found=true", "--wait=false")
    except Exception as e:
        logger.info("Failed to list pool namespaces: %s", e)

    oc("delete", "namespacepool", pool_name, "--ignore-not-found=true")


def cleanup_reservation(res_name):
    """Delete a reservation."""
    logger.info("Cleaning up reservation %s", res_name)
    oc("delete", "namespacereservation", res_name, "--ignore-not-found=true")


def cleanup_namespace(ns_name):
    """Delete a namespace."""
    logger.info("Cleaning up namespace %s", ns_name)
    oc("delete", "namespace", ns_name, "--ignore-not-found=true", "--wait=false")


@pytest.fixture(scope="session", autouse=True)
def cleanup_e2e_resources():
    """Safety net: clean up any e2e-test resources that survive test cleanup."""
    yield
    logger.info("AfterSuite: cleaning up stale e2e-test resources")

    try:
        result = oc_json("get", "namespacereservations")
        for res in result.get("items", []):
            name = res["metadata"]["name"]
            if name.startswith(E2E_PREFIX):
                cleanup_reservation(name)
    except Exception:
        pass

    try:
        result = oc_json("get", "namespacepools")
        for pool in result.get("items", []):
            name = pool["metadata"]["name"]
            if name.startswith(E2E_PREFIX):
                cleanup_pool(name)
    except Exception:
        pass

    cleanup_namespace("e2e-test-secret-source")
