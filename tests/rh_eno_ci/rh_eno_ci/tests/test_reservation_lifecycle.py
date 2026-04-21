"""Tests for NamespaceReservation lifecycle."""

import logging

import pytest
from wait_for import wait_for

from .conftest import (
    cleanup_pool,
    cleanup_reservation,
    create_pool,
    create_reservation,
    oc_json,
    wait_for_pool_ready,
    wait_for_ready_pool_namespace,
    wait_for_reservation_active,
)

logger = logging.getLogger(__name__)

POOL_NAME = "e2e-test-reservation-pool"


@pytest.fixture(scope="module")
def ready_pool():
    """Create a pool and wait for at least one namespace to be ready."""
    create_pool(POOL_NAME, size=2)
    wait_for_ready_pool_namespace(POOL_NAME)
    yield POOL_NAME
    cleanup_pool(POOL_NAME)


class TestReservationLifecycle:
    def test_reservation_becomes_active(self, ready_pool):
        res_name = "e2e-test-res-active"
        create_reservation(res_name, ready_pool)
        ns = wait_for_reservation_active(res_name)
        assert ns.startswith("ephemeral-")
        cleanup_reservation(res_name)

    def test_reserved_namespace_annotated(self, ready_pool):
        res_name = "e2e-test-res-marked"
        create_reservation(res_name, ready_pool)
        ns_name = wait_for_reservation_active(res_name)

        ns = oc_json("get", "namespace", ns_name)
        assert ns["metadata"]["annotations"]["reserved"] == "true"
        cleanup_reservation(res_name)

    def test_expiration_is_set(self, ready_pool):
        res_name = "e2e-test-res-expiry"
        create_reservation(res_name, ready_pool)
        wait_for_reservation_active(res_name)

        res = oc_json("get", "namespacereservation", res_name)
        expiration = res["status"].get("expiration", "")
        assert expiration != "", "expiration should be set"
        cleanup_reservation(res_name)

    def test_namespace_owner_changes_to_reservation(self, ready_pool):
        res_name = "e2e-test-res-owner"
        create_reservation(res_name, ready_pool)
        ns_name = wait_for_reservation_active(res_name)

        ns = oc_json("get", "namespace", ns_name)
        owner_refs = ns["metadata"].get("ownerReferences", [])
        assert len(owner_refs) > 0
        assert owner_refs[0]["kind"] == "NamespaceReservation"
        assert owner_refs[0]["name"] == res_name
        cleanup_reservation(res_name)

    def test_reservation_deleted_after_expiry(self, ready_pool):
        wait_for_pool_ready(POOL_NAME, expected_ready=2)

        res_name = "e2e-test-res-expire"
        create_reservation(res_name, ready_pool, duration="30s")
        wait_for_reservation_active(res_name)

        def _is_deleted():
            try:
                oc_json("get", "namespacereservation", res_name)
                return False
            except Exception:
                return True

        wait_for(_is_deleted, timeout=120, delay=10,
                 message=f"waiting for reservation {res_name} to be deleted after expiry")
