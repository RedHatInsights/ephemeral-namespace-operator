"""Tests for pool status tracking."""

import logging

from .conftest import (
    cleanup_pool,
    cleanup_reservation,
    create_pool,
    create_reservation,
    oc_json,
    wait_for_pool_ready,
    wait_for_reservation_active,
)

logger = logging.getLogger(__name__)


class TestPoolStatus:
    def test_ready_count(self):
        pool_name = "e2e-test-status-pool"
        create_pool(pool_name, size=2)
        try:
            wait_for_pool_ready(pool_name, expected_ready=2)
            pool = oc_json("get", "namespacepool", pool_name)
            assert pool["status"]["ready"] == 2
            assert pool["status"]["creating"] == 0
        finally:
            cleanup_pool(pool_name)

    def test_reserved_count_after_reservation(self):
        pool_name = "e2e-test-reserved-pool"
        res_name = "e2e-test-res-status"
        create_pool(pool_name, size=2)
        try:
            wait_for_pool_ready(pool_name, expected_ready=2)

            create_reservation(res_name, pool_name)
            wait_for_reservation_active(res_name)

            pool = oc_json("get", "namespacepool", pool_name)
            assert pool["status"].get("reserved", 0) >= 1
        finally:
            cleanup_reservation(res_name)
            cleanup_pool(pool_name)
