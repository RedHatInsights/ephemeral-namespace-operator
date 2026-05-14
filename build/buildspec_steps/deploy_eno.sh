set -e

ENO_NAMESPACE="ephemeral-namespace-operator-system"

echo "Deploying ENO operator..."
kubectl create namespace ${ENO_NAMESPACE} || true
kubectl apply -f manifest.yaml --validate=false -n ${ENO_NAMESPACE}

echo "=== Debugging ENO deployment ==="
sleep 10
kubectl get pods -n ${ENO_NAMESPACE} -o wide
kubectl get deployment -n ${ENO_NAMESPACE}
kubectl describe deployment ephemeral-namespace-operator-controller-manager -n ${ENO_NAMESPACE} || true
kubectl get events -n ${ENO_NAMESPACE} --sort-by='.lastTimestamp' | tail -20 || true

ENO_POD=$(kubectl get pod -n ${ENO_NAMESPACE} -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$ENO_POD" ]; then
    echo "=== ENO pod found: $ENO_POD ==="
    kubectl describe pod "$ENO_POD" -n ${ENO_NAMESPACE} || true
    kubectl logs "$ENO_POD" -n ${ENO_NAMESPACE} --tail=50 || true
else
    echo "=== No ENO pod found yet ==="
fi

echo "=== Starting rollout wait ==="
kubectl rollout status deployment/ephemeral-namespace-operator-controller-manager -n ${ENO_NAMESPACE} --timeout=600s
