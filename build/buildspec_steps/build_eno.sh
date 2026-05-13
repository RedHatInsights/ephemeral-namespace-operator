set -e

echo "Building ENO image locally..."

export PATH="/usr/local/go/bin:$PATH"

export IMAGE_TAG=$(git rev-parse --short=8 HEAD)
export IMG="ephemeral-namespace-operator:${IMAGE_TAG}"
docker build -t ${IMG} .
kind load docker-image ${IMG} --name ${CLUSTER_NAME}

echo "Generating manifest with local image ${IMG}..."
make release
