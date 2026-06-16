# Ephemeral Namespace Operator (ENO)

A Kubernetes operator that manages pools of pre-provisioned, ready-to-use OpenShift namespaces with
deployed ClowdEnvironments. Users request namespaces via reservations, and the operator handles
lifecycle management including automatic expiration.

## How It Works

The operator maintains pools of namespaces with pre-deployed resources. When a user creates a
reservation, a namespace is checked out from the appropriate pool and made available for the
specified duration (default: 1 hour). After expiration, the namespace is reclaimed and recycled.

### Reservation Flags

- `--name` — specify a reservation name (e.g., `--name my-reservation`)
- `-d, --duration` — reservation duration (e.g., `-d 3h`)
- `--pool` — target pool (e.g., `--pool minimal`)

### Namespace Pools

Different pools provide different pre-deployed resources:

| Pool            | Resources                                             |
| --------------- | ----------------------------------------------------- |
| `default`       | Full ClowdEnvironment (Kafka, Minio, Prometheus, etc) |
| `minimal`       | Minimal ClowdEnvironment                              |
| `managed-kafka` | ClowdEnvironment with managed Kafka                   |

Pools support a `sizeLimit` configuration to cap the total number of namespaces (ready, creating,
and reserved combined).

### CRDs Created Per Namespace

- **ClowdEnvironment** — deployments for Kafka, Kafka Connect, Minio, Prometheus, feature flags
- **FrontendEnvironment** — configuration for the [frontend operator][frontend-operator]
- **RoleBindings** — edit access for developers
- **Secrets** — copied from the base namespace

## Prerequisites

- Go 1.25+
- OpenShift cluster with Clowder installed
- `make` for build tooling

## Development Setup

```sh
# Run tests
make test

# Run linter
make lint

# Build the operator
make build

# Generate manifests
make manifests

# Generate deep copy implementations
make generate
```

### CI/CD

The repository uses GitHub Actions for:

- `lint.yml` — runs golangci-lint
- `test.yml` — runs unit tests
- `pr-e2e-codebuild.yml` — end-to-end tests via AWS CodeBuild
- `platsec.yml` — platform security scanning

A Jenkinsfile is also present for legacy CI integration.

## Contributing

See [CONTRIBUTING.md][contributing] for local development setup and contribution guidelines.

## License

Apache License 2.0

[frontend-operator]: https://github.com/RedHatInsights/frontend-operator
[contributing]: ./CONTRIBUTING.md
