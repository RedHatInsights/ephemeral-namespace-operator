set -e

echo "Installing static CRDs (ClowdEnvironment, FrontendEnvironment)..."
kubectl apply -f config/crd/static/

echo "Installed CRDs:"
kubectl get crds | grep cloud.redhat.com || true
