set -e

echo "Creating kind cluster ${CLUSTER_NAME} using ${KINDEST_NODE_IMAGE}..."

cat > kind.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        enable-admission-plugins: ValidatingAdmissionWebhook,MutatingAdmissionWebhook
EOF

kind create cluster --name "${CLUSTER_NAME}" --image "${KINDEST_NODE_IMAGE}" --config kind.yaml --wait 180s
kubectl cluster-info
kubectl get nodes -o wide
