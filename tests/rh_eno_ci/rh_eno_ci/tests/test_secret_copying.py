"""Tests for secret copying with annotation filtering."""

import json
import logging

import pytest

from .conftest import (
    cleanup_namespace,
    cleanup_pool,
    create_pool,
    oc,
    oc_json,
    wait_for_ready_pool_namespace,
)

logger = logging.getLogger(__name__)

POOL_NAME = "e2e-test-secret-pool"
SECRET_SRC_NS = "e2e-test-secret-source"


@pytest.fixture(scope="module")
def pool_namespace():
    """Create source namespace with test secrets, then a pool that copies from it."""
    # Create source namespace
    oc("create", "namespace", SECRET_SRC_NS)

    secrets = [
        {
            "apiVersion": "v1", "kind": "Secret",
            "metadata": {
                "name": "vault-managed-secret",
                "namespace": SECRET_SRC_NS,
                "annotations": {"qontract.integration": "openshift-vault-secrets"},
            },
            "type": "Opaque",
            "stringData": {"api-key": "test-api-key", "password": "test-password"},
        },
        {
            "apiVersion": "v1", "kind": "Secret",
            "metadata": {
                "name": "rhcs-cert-secret",
                "namespace": SECRET_SRC_NS,
                "annotations": {"qontract.integration": "openshift-rhcs-certs"},
            },
            "type": "Opaque",
            "stringData": {"cert.pem": "test-cert-data"},
        },
        {
            "apiVersion": "v1", "kind": "Secret",
            "metadata": {
                "name": "unmanaged-secret",
                "namespace": SECRET_SRC_NS,
            },
            "type": "Opaque",
            "stringData": {"key": "should-not-be-copied"},
        },
        {
            "apiVersion": "v1", "kind": "Secret",
            "metadata": {
                "name": "ignored-secret",
                "namespace": SECRET_SRC_NS,
                "annotations": {
                    "qontract.integration": "openshift-vault-secrets",
                    "bonfire.ignore": "true",
                },
            },
            "type": "Opaque",
            "stringData": {"key": "should-be-ignored"},
        },
    ]

    for secret in secrets:
        oc("apply", "-f", "-", _in=json.dumps(secret))

    create_pool(POOL_NAME, size=1, secret_source_ns=SECRET_SRC_NS)
    ns_name = wait_for_ready_pool_namespace(POOL_NAME)

    yield ns_name

    cleanup_pool(POOL_NAME)
    cleanup_namespace(SECRET_SRC_NS)


class TestSecretCopying:
    def test_vault_managed_secret_copied(self, pool_namespace):
        secret = oc_json("get", "secret", "vault-managed-secret", "-n", pool_namespace)
        assert secret["metadata"]["name"] == "vault-managed-secret"

    def test_vault_managed_secret_data_preserved(self, pool_namespace):
        secret = oc_json("get", "secret", "vault-managed-secret", "-n", pool_namespace)
        assert "api-key" in secret["data"]

    def test_rhcs_cert_secret_copied(self, pool_namespace):
        secret = oc_json("get", "secret", "rhcs-cert-secret", "-n", pool_namespace)
        assert secret["metadata"]["name"] == "rhcs-cert-secret"

    def test_unmanaged_secret_not_copied(self, pool_namespace):
        try:
            oc_json("get", "secret", "unmanaged-secret", "-n", pool_namespace)
            pytest.fail("unmanaged-secret should not be copied")
        except Exception:
            pass

    def test_bonfire_ignored_secret_not_copied(self, pool_namespace):
        try:
            oc_json("get", "secret", "ignored-secret", "-n", pool_namespace)
            pytest.fail("ignored-secret should not be copied")
        except Exception:
            pass
