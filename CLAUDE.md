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

## Managing Red Hat Konflux Dependency Updates

The `red-hat-konflux` bot regularly opens PRs to update Go dependencies and Docker base images. These PRs typically update `go.mod`, `go.sum`, and occasionally `Dockerfile`.

### Strategy 1: Merge Individual PRs (Preferred for Small Batches)

When there are only a few open konflux PRs, use the `gh` CLI to merge them with admin privileges:

```bash
# List all open konflux PRs
gh pr list --author "app/red-hat-konflux" --json number,title,mergeable,state --state open

# Attempt to merge all PRs without conflicts
for pr in 1447 1448 1449 ...; do
  gh pr merge $pr --squash --delete-branch --admin
done
```

**Note**: PRs that are behind the base branch or have conflicts will fail to merge and need conflict resolution.

### Strategy 2: Combine into Single PR (For Large Batches)

When there are 15+ open konflux PRs, combine them into a single PR:

1. **Create a combined branch**:
   ```bash
   git checkout -b combined-konflux-dependency-updates
   ```

2. **Fetch all remote branches**:
   ```bash
   git fetch origin
   ```

3. **Merge all konflux branches sequentially**:
   ```bash
   # Get list of branch names from PRs (excluding abandoned ones)
   gh pr list --author "app/red-hat-konflux" --json headRefName --state open | jq -r '.[].headRefName'

   # Merge each branch, resolving conflicts as needed
   for branch in <list-of-branches>; do
     git merge --no-edit "origin/$branch" || break
   done
   ```

4. **Resolve merge conflicts**:
   - For dependency conflicts in `go.mod`, choose the newer version
   - For `go.sum`, use `git checkout --ours go.sum` to keep your accumulated changes
   - Common conflict pattern: Multiple PRs updating related dependencies (e.g., golang.org/x packages)
   - When choosing versions:
     - Compare version numbers (e.g., v0.34.0 > v0.32.0)
     - For digest-based versions, compare dates in the commit hash (e.g., 20260202 > 20260115)

5. **Important: Preserve critical dependencies**:
   - **Always keep `rhc-osdk-utils` at the version specified in the latest PR** (currently v0.14.0)
   - If a merge downgrades this package, manually revert it back

6. **Verify Go version alignment**:
   - The version in `go.mod` must match the go-toolset image in `Dockerfile`
   - Example: If `go.mod` has `go 1.25.0`, `Dockerfile` should use `registry.access.redhat.com/ubi8/go-toolset:1.25.x-*`
   - Check available images at https://catalog.redhat.com/software/containers/ubi8/go-toolset
   - Use tags with format `<go version>-<timestamp>` (e.g., `1.25.5-1769026830`)
   - **IMPORTANT**: Update go.mod to match the exact minor version in Dockerfile (e.g., if Dockerfile has 1.25.5, update go.mod from 1.25.0 to 1.25.5)

7. **CRITICAL: Verify the merge is clean before pushing**:
   ```bash
   # Check for any remaining merge conflict markers in go.mod
   grep -E "^(<<<<<<<|=======|>>>>>>>)" go.mod && echo "ERROR: Merge conflicts still present!" || echo "✓ No conflicts"

   # Verify go mod tidy works without errors
   go mod tidy
   if [ $? -eq 0 ]; then
     echo "✓ go mod tidy succeeded"
   else
     echo "ERROR: go mod tidy failed - check for incompatible dependencies"
     # Common issue: cluster-api v1.12.2+ is incompatible with clowder v0.100.0
     # Revert cluster-api to v1.7.2 if needed
     exit 1
   fi

   # Update lint GitHub Action to match go.mod version
   GO_VERSION=$(grep "^go " go.mod | awk '{print $2}')
   echo "Go version in go.mod: $GO_VERSION"

   # Check if .github/workflows/lint.yml needs updating
   if [ -f .github/workflows/lint.yml ]; then
     LINT_GO_VERSION=$(grep "go-version:" .github/workflows/lint.yml | head -1 | awk '{print $2}' | tr -d "'\"")
     if [ "$GO_VERSION" != "$LINT_GO_VERSION" ]; then
       echo "WARNING: Lint workflow uses Go $LINT_GO_VERSION but go.mod specifies $GO_VERSION"
       echo "Update .github/workflows/lint.yml to use go-version: '$GO_VERSION'"
       # Update the file
       sed -i "s/go-version: .*/go-version: '$GO_VERSION'/" .github/workflows/lint.yml
       git add .github/workflows/lint.yml
       echo "✓ Updated lint.yml to Go $GO_VERSION"
     else
       echo "✓ Lint workflow Go version matches go.mod"
     fi
   fi

   # Commit the go mod tidy changes and lint.yml update if any
   git add go.mod go.sum
   git commit -m "Run go mod tidy and verify dependencies" || echo "No changes from go mod tidy"
   ```

8. **Push and create PR**:
   ```bash
   git push -u origin combined-konflux-dependency-updates
   gh pr create --title "chore(deps): Combined dependency updates from red-hat-konflux" \
     --body "<detailed summary of all updates>"
   ```

9. **Wait for individual PRs to auto-close**:
   - After the combined PR is merged, wait ~1 minute
   - GitHub will automatically close most/all of the individual konflux PRs since their changes are now in main
   - Check if any PRs remain open: `gh pr list --author "app/red-hat-konflux" --state open`
   - Only manually close PRs that didn't auto-close:
     ```bash
     gh pr close <pr-number> --comment "Closing - changes included in combined PR #<combined-pr-number>"
     ```

### Handling Merge Conflicts in Konflux PRs

When PRs have conflicts (typically after other PRs have been merged):

1. **Checkout the PR branch**:
   ```bash
   gh pr checkout <pr-number>
   ```

2. **Merge main into the PR branch**:
   ```bash
   git merge origin/main
   ```

3. **Resolve conflicts**:
   - **For `go.mod`**: Choose the newer version of each dependency
   - **For `go.sum`**: Use `git checkout --theirs go.sum` to take main's version
   - **Pattern**: When choosing between versions, use the higher version number or more recent timestamp

4. **Commit and push**:
   ```bash
   git add go.mod go.sum
   git commit -m "Merge main into PR #<pr-number> and resolve conflicts"
   git push origin <branch-name>
   ```

5. **Wait a few seconds, then merge**:
   ```bash
   sleep 3
   gh pr merge <pr-number> --squash --delete-branch --admin
   ```

### Workflow for Batch Merging Conflicted PRs

When multiple PRs have conflicts, resolve them **in order from oldest to newest**:

```bash
# Work through PRs sequentially (oldest first)
for pr in 1455 1456 1457 1459 1462 1463 1464; do
  echo "Processing PR #$pr"

  # Update local main
  git checkout main && git pull origin main

  # Checkout PR and merge main
  gh pr checkout $pr
  git merge origin/main

  # Resolve conflicts (manual step)
  # Edit go.mod to choose newer versions
  git checkout --theirs go.sum  # Use main's go.sum

  # Commit and push
  git add go.mod go.sum
  git commit -m "Merge main into PR #$pr and resolve conflicts"
  git push origin <branch-name>

  # Merge the PR
  sleep 3
  gh pr merge $pr --squash --delete-branch --admin
done
```

**Why oldest first?**: Earlier PRs may update dependencies that later PRs also touch. Merging in order minimizes cascading conflicts.

### Important Considerations

- **CRITICAL: Always verify clean merge before pushing**:
  - Check for remaining merge conflict markers: `grep -E "^(<<<<<<<|=======|>>>>>>>)" go.mod`
  - Run `go mod tidy` to verify all dependencies are compatible and go.sum is correct
  - Update `.github/workflows/lint.yml` to match the Go version in go.mod
  - Commit any changes from `go mod tidy` before pushing

- **go.mod version**: The Konflux bot may try to update the go version before a Docker image is available. Always verify:
  - Version in `go.mod` matches the exact minor version in `Dockerfile` (e.g., 1.25.5, not just 1.25.0)
  - Check for newer images at https://catalog.redhat.com/software/containers/ubi8/go-toolset
  - Use tags with format `<go version>-<timestamp>` (e.g., `1.25.5-1769026830`)
  - If no compatible image exists, reject the go version update

- **Dependency compatibility issues**:
  - **sigs.k8s.io/cluster-api**: Versions 1.12.0+ are incompatible with `clowder v0.100.0`
  - If `go mod tidy` fails with "does not contain package sigs.k8s.io/cluster-api/api/v1beta1", revert cluster-api to v1.7.2
  - Always test that `go mod tidy` runs successfully before pushing

- **rhc-osdk-utils**: Never allow this to be downgraded - always verify it stays at the latest version (currently v0.14.0)

- **Conflict resolution strategy**: When in doubt, choose the newer version of dependencies, but verify compatibility with `go mod tidy`

- **Testing**: Konflux PRs are dependency updates and typically don't require local testing beyond verifying `go mod tidy` works

- **Admin flag**: The `--admin` flag bypasses branch protection rules - use only for automated dependency updates

- **Timing**: Some PRs need a few seconds after pushing before GitHub recognizes them as mergeable
