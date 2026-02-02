# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Ephemeral Namespace Operator (ENO) is a Kubernetes operator built using Kubebuilder that manages pools of pre-provisioned namespaces for the Red Hat Insights platform. It maintains ready-to-use namespaces with ClowdEnvironments and other resources deployed, reducing wait times for developers requesting ephemeral environments.

## Core Architecture

### Custom Resource Definitions (CRDs)

The operator manages two primary CRDs located in `apis/cloud.redhat.com/v1alpha1/`:

1. **NamespacePool** (`namespacepool_types.go`): Defines pools of ready namespaces
   - Contains ClowdEnvironment spec, LimitRange, ResourceQuotas, and team-specific secrets
   - Tracks pool status: ready, creating, and reserved namespace counts
   - Optional `sizeLimit` field controls maximum total namespaces (ready + creating + reserved)
   - Pool types: `default`, `minimal`, `managed-kafka`

2. **NamespaceReservation** (`namespacereservation_types.go`): Represents a user's request for a namespace
   - Users request via `--name`, `--duration` (default 1 hour), and `--pool` flags
   - Status includes expiration time, state (active/waiting/error), and assigned namespace

### Controllers

Three reconcilers in `controllers/cloud.redhat.com/`:

1. **NamespacePoolReconciler** (`namespacepool_controller.go`): Maintains desired pool size
   - Creates namespaces with random 6-character suffixes (`ephemeral-XXXXXX`)
   - Deploys ClowdEnvironment, FrontendEnvironment, RoleBindings per namespace
   - Copies secrets from `ephemeral-base` namespace (including Quay pull secrets)
   - Respects pool `sizeLimit` when creating new namespaces
   - Helper logic in `controllers/cloud.redhat.com/helpers/pool.go` calculates namespace quantity deltas

2. **NamespaceReservationReconciler** (`namespacereservation_controller.go`): Assigns namespaces to users
   - Selects available namespace from requested pool
   - Sets expiration time and updates pool metrics
   - Works with Poller to track active reservations

3. **ClowdenvironmentReconciler** (`clowdenvironment_controller.go`): Reconciles ClowdEnvironment resources within ephemeral namespaces

### Poller Component

`controllers/cloud.redhat.com/poller.go` runs as a background goroutine:
- Checks every 10 seconds for expired reservations
- Automatically deletes expired NamespaceReservation resources
- Maintains `activeReservations` map tracking expiration times
- Launched via `go poller.Poll()` in `run.go`

### Dependencies

The operator integrates with:
- **Clowder**: Cloud.redhat.com application deployment framework (provides ClowdEnvironment CRD)
- **Frontend Operator**: Manages FrontendEnvironment resources
- **OpenShift APIs**: Uses ProjectRequest for namespace creation

## Common Development Commands

### Building and Running

```bash
# Build the operator binary
make build

# Run locally (applies CRDs, generates code, runs operator)
make run

# Run tests with coverage
make test

# Format and vet code
make fmt
make vet

# Generate manifests (CRDs, RBAC, webhooks)
make manifests

# Generate DeepCopy methods
make generate
```

### Container Images

```bash
# Build container image (runs tests)
make docker-build

# Build without tests (quick iteration)
make docker-build-no-test-quick

# Push to registry
make docker-push

# Build and push for Minikube
make deploy-minikube-quick
```

### Deployment

```bash
# Install CRDs to cluster
make install

# Deploy operator to cluster
make deploy

# Uninstall CRDs
make uninstall

# Undeploy operator
make undeploy

# Generate deployment template
make build-template
```

### Testing with Minikube

See CONTRIBUTING.md for detailed local development setup. Quick reference:

1. Start Minikube: `minikube start --cpus 4 --disk-size 36GB --memory 8192MB --addons registry --addons ingress --addons metrics-server`
2. Create `ephemeral-base` namespace and add Quay pull secret
3. Install Clowder: `./build/kube_setup.sh && make deploy-minikube-quick` (from Clowder repo)
4. Apply FrontendEnvironment CRD: `oc apply -f config/crd/static/cloud.redhat.com_frontendenvironments.yaml`
5. Run operator: `make run`
6. Edit `test-pool-spec.yaml` to add your Quay pull secret name
7. Apply pool spec: `oc apply -f test-pool-spec.yaml`

### Running Single Tests

```bash
# Run specific test
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.26.0 --bin-dir bin -p path)" \
  go test ./controllers/cloud.redhat.com -run TestNamespacePoolController -v

# Run tests in a specific package
go test ./controllers/cloud.redhat.com/helpers/... -v
```

## Code Structure

- `main.go`: Entry point, parses flags and calls `controllers.Run()`
- `controllers/cloud.redhat.com/run.go`: Sets up manager, registers controllers, starts poller
- `controllers/cloud.redhat.com/metrics.go`: Prometheus metrics definitions
- `controllers/cloud.redhat.com/helpers/`: Shared logic for namespace, pool, reservation, ClowdEnv, and frontend operations
- `config/`: Kustomize configurations for CRDs, RBAC, deployment manifests
- `pr_check.sh`: CI script for pull request validation

## Important Notes

- The operator uses `podman` by default, falls back to `docker` if unavailable
- Version information is embedded from `controllers/cloud.redhat.com/version.txt`
- The `ephemeral-base` namespace is critical - secrets are copied from here to all ephemeral namespaces
- Secrets with type `rhcs-certs` are automatically copied to ephemeral namespaces
- Use `oc` or `kubectl` interchangeably for cluster operations
- Pool calculations in `helpers/pool.go` handle `sizeLimit` constraints to prevent over-provisioning
