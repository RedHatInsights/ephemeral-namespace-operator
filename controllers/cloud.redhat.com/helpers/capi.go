package helpers

import (
	"context"
	"fmt"
	"time"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// stuckDeletionThreshold is how long a ROSAMachinePool must have been in deletion before its
// finalizer is force-removed. This covers transient OCM errors (e.g. 429 rate-limiting) that
// the controller cannot recover from on its own within a reasonable window.
const stuckDeletionThreshold = time.Hour

// RemoveStuckROSAMachinePoolFinalizers removes the controller finalizer from ROSAMachinePool
// resources that have been stuck in deletion for longer than stuckDeletionThreshold, and whose
// owning Cluster confirms it is blocked waiting on that MachinePool deletion. Both conditions
// must hold.
//
// Returns (false, nil) when the ROSAMachinePool CRD is not installed on the cluster.
func RemoveStuckROSAMachinePoolFinalizers(ctx context.Context, cl client.Client, namespace string) (bool, error) {
	const (
		finalizer        = "rosamachinepools.infrastructure.cluster.x-k8s.io"
		clusterNameLabel = "cluster.x-k8s.io/cluster-name"
	)

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

		// Only act when deletion has been stuck long enough to rule out transient errors.
		if time.Since(pool.GetDeletionTimestamp().Time) < stuckDeletionThreshold {
			continue
		}

		// Also verify the owning Cluster is blocked specifically on MachinePool deletion.
		clusterName := pool.GetLabels()[clusterNameLabel]
		if clusterName == "" {
			continue
		}
		waiting, err := clusterWaitingForWorkersDeletion(ctx, cl, namespace, clusterName)
		if err != nil {
			return false, err
		}
		if !waiting {
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

// clusterWaitingForWorkersDeletion returns true when the named Cluster's Deleting condition
// indicates it is blocked waiting for MachinePool deletion to complete, or when the Cluster
// itself no longer exists (the entire ownership chain above the pool is gone, so no controller
// will ever reconcile the pool's finalizer away).
func clusterWaitingForWorkersDeletion(ctx context.Context, cl client.Client, namespace, clusterName string) (bool, error) {
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta2",
		Kind:    "Cluster",
	})
	if err := cl.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: namespace}, cluster); err != nil {
		if k8serr.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("could not get Cluster [%s/%s]: %w", namespace, clusterName, err)
	}

	conditions, _, _ := unstructured.NestedSlice(cluster.Object, "status", "conditions")
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condReason, _ := cond["reason"].(string)
		condStatus, _ := cond["status"].(string)
		if condType == "Deleting" && condReason == "WaitingForWorkersDeletion" && condStatus == "True" {
			return true, nil
		}
	}
	return false, nil
}
