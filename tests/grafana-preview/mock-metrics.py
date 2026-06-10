"""Mock Prometheus metrics exporter for the ENO Grafana dashboard.

Serves realistic fake data for all metrics referenced in the dashboard so
panels render with meaningful values when running locally.
"""

import random
import string
import time
import threading
from prometheus_client import (
    Counter,
    Gauge,
    Histogram,
    start_http_server,
    REGISTRY,
)

# -- Configuration ----------------------------------------------------------

POOLS = ["default", "minimal", "minimal-secure", "managed-kafka"]
HUMAN_REQUESTERS = [
    "jdoe", "asmith", "mjones",
]
# CI job requesters with trailing UIDs (the dashboard strips the last segment)
CI_REQUESTER_PREFIXES = [
    "insights-hbi-smoke",
    "insights-hbi-rbac",
    "insights-hbi-all",
    "bonfire-component-tests-pipelinerun",
    "bonfire-integration-tests-pipelinerun",
    "rbac-bonfire-tekton",
    "compliance-backend-bonfire-tekton",
    "koku-ci",
    "content-sources-backend-bonfire-tekton",
]
CONTROLLERS = ["clowdenvironment", "namespacepool", "namespacereservation"]
REST_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "LIST", "WATCH"]
WORKQUEUE_NAMES = ["clowdenvironment", "namespacepool", "namespacereservation"]
VERSION = "0.42.0"

# -- Metrics ----------------------------------------------------------------

reservations_by_requester = Counter(
    "reservations_by_requester_total",
    "Total namespace reservations by requester and pool",
    ["requester", "pool"],
)

active_reservations = Gauge(
    "active_reservation_total",
    "Total active reservations",
    ["controller"],
)

failed_pool_reservations = Gauge(
    "failed_pool_reservations_total",
    "Total failed reservations from each pool",
    ["pool"],
)

eno_version = Gauge(
    "eno_version",
    "ENO version (1 if present)",
    ["version"],
)

reservation_duration = Histogram(
    "namespace_reservation_duration_average",
    "Average duration for namespace reservations (in hours)",
    ["controller", "pool"],
    buckets=[1, 2, 4, 8, 24, 48, 168, 336],
)

namespace_creation = Histogram(
    "namespace_creation_average_seconds",
    "Average time namespace creation occurs",
    ["pool"],
    buckets=[1, 2, 3, 4, 5, 7, 14, 28, 56, 112, 224],
)

reconcile_total = Counter(
    "controller_runtime_reconcile_total",
    "Reconciliation count by controller",
    ["controller"],
)

workqueue_depth = Gauge(
    "workqueue_depth",
    "Workqueue depth",
    ["name", "service"],
)

rest_client_requests = Counter(
    "rest_client_requests_total",
    "REST client request count",
    ["method"],
)

container_memory = Gauge(
    "container_memory_working_set_bytes",
    "Container memory working set",
    ["namespace", "container"],
)

go_memstats_alloc = Gauge(
    "go_memstats_alloc_bytes",
    "Go allocated memory",
    ["container", "namespace"],
)

container_cpu = Counter(
    "container_cpu_usage_seconds_total",
    "Container CPU usage",
    ["container", "namespace"],
)

cpu_requests = Gauge(
    "kube_pod_container_resource_requests_cpu_cores",
    "CPU resource requests",
    ["container", "namespace"],
)

capi_cleanup_duration = Gauge(
    "capi_cleanup_duration_seconds",
    "Time since namespace entered CAPI cleanup state",
    ["namespace", "reservation"],
)


def random_uid():
    return "".join(random.choices(string.ascii_lowercase + string.digits, k=5))


def random_requester():
    if random.random() < 0.15:
        return random.choice(HUMAN_REQUESTERS)
    return f"{random.choice(CI_REQUESTER_PREFIXES)}-{random_uid()}"


# -- Seed initial state -----------------------------------------------------

def seed():
    eno_version.labels(version=VERSION).set(1)

    for pool in POOLS:
        failed_pool_reservations.labels(pool=pool).set(random.randint(0, 3))

    for _ in range(80):
        req = random_requester()
        pool = random.choice(POOLS)
        reservations_by_requester.labels(requester=req, pool=pool).inc(1)

    active_reservations.labels(controller="namespacereservation").set(
        random.randint(3, 15)
    )

    for ctrl in CONTROLLERS:
        reconcile_total.labels(controller=ctrl).inc(random.randint(50, 500))

    container_memory.labels(
        namespace="ephemeral-namespace-operator-system", container="manager"
    ).set(random.uniform(80e6, 200e6))

    go_memstats_alloc.labels(
        container="manager", namespace="ephemeral-namespace-operator-system"
    ).set(random.uniform(30e6, 80e6))

    cpu_requests.labels(
        container="manager", namespace="ephemeral-namespace-operator-system"
    ).set(0.5)

    for method in REST_METHODS:
        rest_client_requests.labels(method=method).inc(random.randint(10, 500))

    for name in WORKQUEUE_NAMES:
        workqueue_depth.labels(
            name=name,
            service="ephemeral-namespace-operator-controller-metrics-non-auth",
        ).set(random.randint(0, 5))

    for pool in POOLS:
        for _ in range(random.randint(10, 40)):
            hours = random.choice([1, 1, 1, 2, 4, 8, 24, 48])
            reservation_duration.labels(
                controller="namespacereservation", pool=pool
            ).observe(hours)
            namespace_creation.labels(pool=pool).observe(
                random.uniform(2, 30)
            )


# -- Continuous updates (makes timeseries interesting) ----------------------

def update_loop():
    while True:
        time.sleep(5)

        # Simulate new reservations trickling in (with unique CI UIDs)
        req = random_requester()
        pool = random.choice(POOLS)
        reservations_by_requester.labels(requester=req, pool=pool).inc(1)

        # Fluctuate active reservations
        active_reservations.labels(controller="namespacereservation").set(
            max(0, active_reservations.labels(
                controller="namespacereservation"
            )._value.get() + random.choice([-1, 0, 0, 1, 1, 2]))
        )

        # Reconciliations keep happening
        for ctrl in CONTROLLERS:
            reconcile_total.labels(controller=ctrl).inc(
                random.randint(0, 3)
            )

        # Memory jitter
        container_memory.labels(
            namespace="ephemeral-namespace-operator-system", container="manager"
        ).set(random.uniform(80e6, 200e6))
        go_memstats_alloc.labels(
            container="manager", namespace="ephemeral-namespace-operator-system"
        ).set(random.uniform(30e6, 80e6))

        # CPU
        container_cpu.labels(
            container="manager", namespace="ephemeral-namespace-operator-system"
        ).inc(random.uniform(0.001, 0.01))

        # REST calls
        rest_client_requests.labels(method=random.choice(REST_METHODS)).inc(
            random.randint(1, 5)
        )

        # Workqueue depth fluctuation
        for name in WORKQUEUE_NAMES:
            workqueue_depth.labels(
                name=name,
                service="ephemeral-namespace-operator-controller-metrics-non-auth",
            ).set(random.randint(0, 5))

        # Record reservation duration (drives 'Reservations per Hour by Pool')
        if random.random() < 0.6:
            pool = random.choice(POOLS)
            hours = random.choice([1, 1, 2, 4, 8, 24])
            reservation_duration.labels(
                controller="namespacereservation", pool=pool
            ).observe(hours)
            namespace_creation.labels(pool=pool).observe(
                random.uniform(2, 30)
            )


if __name__ == "__main__":
    seed()
    threading.Thread(target=update_loop, daemon=True).start()
    print("Mock metrics server starting on :8000")
    start_http_server(8000)

    # Block forever
    threading.Event().wait()
