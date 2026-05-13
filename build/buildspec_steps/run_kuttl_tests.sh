set -e

ENO_NAMESPACE="ephemeral-namespace-operator-system"

echo "Running KUTTL tests..."
set +e
bash build/run_kuttl.sh --report xml
TEST_RC=$?
set -e
mv "${ARTIFACTS_DIR}/kuttl-report.xml" "${ARTIFACTS_DIR}/junit-kuttl.xml" || true

echo "Collecting logs and metrics..."
for p in $(kubectl get pod -n ${ENO_NAMESPACE} -o jsonpath='{.items[*].metadata.name}'); do
    kubectl logs "$p" -n ${ENO_NAMESPACE} > "${ARTIFACTS_DIR}/${p}.log" || true
    kubectl logs "$p" -n ${ENO_NAMESPACE} --previous=true > "${ARTIFACTS_DIR}/${p}-previous.log" || true
done
kubectl -n ${ENO_NAMESPACE} get all -o wide > "${ARTIFACTS_DIR}/eno-system-get-all.txt" || true
kubectl get events --all-namespaces --sort-by=.lastTimestamp > "${ARTIFACTS_DIR}/cluster-events.txt" || true

exit "$TEST_RC"
