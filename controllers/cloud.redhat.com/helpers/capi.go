package helpers

import (
	"context"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteCAPIResources deletes all Cluster resources in the given namespace and reports whether any remain.
// The caller should requeue if this returns true, allowing CAPI controllers time to run their
// finalizers (which require rosa-creds-secret to be present) before the namespace is GC'd.
func DeleteCAPIResources(ctx context.Context, cl client.Client, namespace string) (bool, error) {
	clusterList := clusterv1.ClusterList{}
	if err := cl.List(ctx, &clusterList, client.InNamespace(namespace)); err != nil {
		return false, fmt.Errorf("could not list Cluster resources in [%s]: %w", namespace, err)
	}

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		if cluster.DeletionTimestamp.IsZero() {
			if err := cl.Delete(ctx, cluster); err != nil && !k8serr.IsNotFound(err) {
				return false, fmt.Errorf("could not delete Cluster [%s/%s]: %w", namespace, cluster.Name, err)
			}
		}
	}

	return len(clusterList.Items) > 0, nil
}
