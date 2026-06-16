# Ephemeral Namespace Operator

## Project Overview

A Kubernetes operator that manages pools of pre-provisioned OpenShift namespaces with deployed
ClowdEnvironments. Users request namespaces via reservations; the operator handles provisioning,
checkout, expiration, and recycling. Built with the Operator SDK and deployed on OpenShift.

## Dependencies

- **Runtime:** Go 1.25+, controller-runtime, Clowder CRDs, frontend-operator CRDs
- **Build:** Make, Docker/Podman
- **Test:** Ginkgo/Gomega, envtest (kubebuilder assets)
- **Lint:** golangci-lint
- **CI:** GitHub Actions (lint, test, e2e via CodeBuild, platsec), Jenkinsfile (legacy)

## Development Commands

```sh
# Run tests (includes fmt and vet)
make test

# Run linter (via podman)
make lint

# Build the operator binary
make build

# Generate CRD manifests
make manifests

# Generate DeepCopy implementations
make generate
```

See [Development Setup][readme-dev] in the README for full prerequisites and setup instructions.

## Architecture

The operator uses a pool-and-reservation model. Controllers reconcile `NamespacePool` and
`NamespaceReservation` custom resources, pre-provisioning namespaces with ClowdEnvironments and
managing checkout/expiration lifecycle. See [ARCHITECTURE.md][architecture] for detailed design
decisions and module structure.

## Code Style

- **Linter:** golangci-lint (configured via `.golangci.yml`)
- **Test framework:** Ginkgo v2 with Gomega matchers
- **Go version:** 1.25+ (per `go.mod`)
- Standard Operator SDK project layout (controllers, apis, config directories)

## Common Mistakes

1. **Running `make lint` without Podman.** The lint target runs golangci-lint inside a Podman
   container. If Podman is not installed or the Docker socket is not available, the lint step
   will fail silently or with a confusing error.

2. **Forgetting to run `make generate` after CRD changes.** Modifying types in `apis/` requires
   regenerating DeepCopy methods. Tests will fail with nil pointer dereferences if DeepCopy
   implementations are out of date.

3. **Confusing pool types.** The `default`, `minimal`, and `managed-kafka` pools have different
   resource configurations. Tests that assume a specific pool's resources will fail when run
   against a different pool type.

## Testing

```sh
# Unit tests with envtest
make test

# E2E tests (requires AWS CodeBuild or local cluster)
# Triggered via GitHub Actions on PR
```

Tests use envtest to spin up a local API server with the operator's CRDs installed.

[readme-dev]: ./README.md#development-setup
[architecture]: ./ARCHITECTURE.md
