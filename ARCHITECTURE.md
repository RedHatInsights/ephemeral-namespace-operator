# Architecture

## Overview

The Ephemeral Namespace Operator (ENO) is a Kubernetes operator built with the Operator SDK
(controller-runtime). It manages namespace lifecycle through a pool-and-reservation model where
namespaces are pre-provisioned with ClowdEnvironments and checked out on demand.

## Module Structure

```text
main.go                  # Operator entry point, manager setup
controllers/             # Reconciliation logic for custom resources
apis/                    # CRD type definitions (NamespaceReservation, NamespacePool, etc.)
config/                  # Kustomize manifests for CRDs, RBAC, and deployment
hack/                    # Development scripts and utilities
build/                   # Container build scripts
ci/                      # CI-specific configuration
docs/                    # Additional documentation
```

## Key Dependencies

- **controller-runtime** — core operator framework for reconciliation loops
- **Clowder** (`github.com/RedHatInsights/clowder`) — ClowdEnvironment CRD definitions
- **frontend-operator** (`github.com/RedHatInsights/frontend-operator`) — FrontendEnvironment CRDs
- **rhc-osdk-utils** (`github.com/RedHatInsights/rhc-osdk-utils`) — shared operator SDK utilities
- **cluster-api** (`sigs.k8s.io/cluster-api`) — cluster lifecycle types

## Reconciliation Flow

The operator runs multiple controllers that watch and reconcile custom resources:

1. **NamespacePool controller** — maintains the desired count of ready namespaces per pool,
   creating new ones when the pool drops below its target and cleaning up expired ones
2. **NamespaceReservation controller** — handles user reservation requests by selecting an
   available namespace from the requested pool, setting expiration timers, and releasing
   namespaces when reservations expire

## Design Decisions

- **Pre-provisioning over on-demand.** Namespaces are created ahead of time to eliminate user
  wait times. The tradeoff is higher baseline resource consumption.
- **Pool-based isolation.** Different pools (`default`, `minimal`, `managed-kafka`) allow users to
  request only the resources they need, reducing waste compared to a single pool with all resources.
- **Size limits.** The optional `sizeLimit` on pools prevents runaway namespace creation in case of
  bugs or abuse.
- **Prometheus metrics.** The operator exposes custom metrics via `prometheus/client_golang` for
  monitoring pool health, reservation counts, and namespace lifecycle events.
