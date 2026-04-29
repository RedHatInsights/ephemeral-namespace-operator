package helpers

import (
	"context"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeleteCAPIResources deletes all Cluster resources in the given namespace and reports whether any remain.
// The caller should requeue if this returns true, allowing CAPI controllers time to run their
// finalizers (which require rosa-creds-secret to be present) before the namespace is GC'd.
// Returns (false, nil) when the Cluster CRD is not installed on the cluster.
func DeleteCAPIResources(ctx context.Context, cl client.Client, namespace string) (bool, error) {
	clusterList := clusterv1.ClusterList{}
	if err := cl.List(ctx, &clusterList, client.InNamespace(namespace)); err != nil {
		if apimeta.IsNoMatchError(err) {
			return false, nil
		}
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

// RemoveStuckROSAMachinePoolFinalizers removes the controller finalizer from any ROSAMachinePool
// resources in the namespace that are stuck in deletion due to a reconciliation failure (e.g. the
// OCM API rejecting an update with 400). This unblocks the MachinePool → Cluster deletion chain.
// Returns (false, nil) when the ROSAMachinePool CRD is not installed on the cluster.
func RemoveStuckROSAMachinePoolFinalizers(ctx context.Context, cl client.Client, namespace string) (bool, error) {
	const finalizer = "rosamachinepools.infrastructure.cluster.x-k8s.io"

	poolList := &unstructured.UnstructuredList{}
	poolList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta2",
		Kind:    "ROSAMachinePoolList",
	})

	if err := cl.List(ctx, poolList, client.InNamespace(namespace)); err != nil {
		if apimeta.IsNoMatchError(err) {
			return false, nil
		}
		return false, fmt.Errorf("could not list ROSAMachinePool in [%s]: %w", namespace, err)
	}

	patched := false
	for i := range poolList.Items {
		pool := &poolList.Items[i]
		if pool.GetDeletionTimestamp().IsZero() || !controllerutil.ContainsFinalizer(pool, finalizer) {
			continue
		}
		controllerutil.RemoveFinalizer(pool, finalizer)
		if err := cl.Update(ctx, pool); err != nil && !k8serr.IsNotFound(err) {
			return false, fmt.Errorf("could not remove finalizer from ROSAMachinePool [%s/%s]: %w", namespace, pool.GetName(), err)
		}
		patched = true
	}

	return patched, nil
}
