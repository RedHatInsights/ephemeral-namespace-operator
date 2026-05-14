set -e

if [ -n "${GITHUB_PR_NUMBER}" ]; then
    echo "=========================================="
    echo "Running E2E tests for PR #${GITHUB_PR_NUMBER}"
    echo "Commit: ${GITHUB_SHA}"
    echo "Branch: ${GITHUB_REF}"
    echo "Triggered by: ${GITHUB_ACTOR}"
    echo "Repository: ${GITHUB_REPOSITORY}"
    echo "=========================================="
fi

echo "Installing Go 1.25.7..."
rm -rf /usr/local/go
curl -fsSL https://go.dev/dl/go1.25.7.linux-amd64.tar.gz | tar -C /usr/local -xzf -
export PATH="/usr/local/go/bin:$PATH"
go version

echo "Installing base tools (jq, git, make, tar, unzip)..."
yum -y install jq git make tar unzip >/dev/null 2>&1 || true

echo "Installing kind, kubectl, and kuttl..."
curl -fsSL -o /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.23.0/kind-linux-amd64
chmod +x /usr/local/bin/kind
curl -fsSL -o /usr/local/bin/kubectl https://dl.k8s.io/release/${K8S_VERSION}/bin/linux/amd64/kubectl
chmod +x /usr/local/bin/kubectl

echo "Installing kubectl-kuttl from GitHub releases..."
curl -fsSL https://github.com/kudobuilder/kuttl/releases/download/v${KUTTL_VERSION}/kubectl-kuttl_${KUTTL_VERSION}_linux_x86_64 -o /usr/local/bin/kubectl-kuttl
chmod +x /usr/local/bin/kubectl-kuttl
kubectl-kuttl version
