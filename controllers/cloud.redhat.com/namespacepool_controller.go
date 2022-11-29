/*
Copyright 2021.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
)

const (
	POOL_STATUS_READY    = "ready"
	POOL_STATUS_CREATING = "creating"
)

// NamespacePoolReconciler reconciles a NamespacePool object
type NamespacePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacepools/finalizers,verbs=update

func (r *NamespacePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pool := crd.NamespacePool{}

	if err := r.Client.Get(ctx, req.NamespacedName, &pool); err != nil {
		r.Log.Error(err, fmt.Sprintf("cannot retrieve [%s] pool resource", pool.Name))
		return ctrl.Result{Requeue: true}, err
	}

	errNamespaceList, err := r.getPoolStatus(ctx, &pool)
	if err != nil {
		r.Log.Error(err, "unable to get status of owned namespaces")
		return ctrl.Result{Requeue: true}, err
	}

	if len(errNamespaceList) > 0 {
		r.Log.Info(fmt.Sprintf("[%d] namespaces in error state are queued for deletion", len(errNamespaceList)))
		err = r.handleErrorNamespaces(ctx, errNamespaceList)
		if err != nil {
			r.Log.Error(err, "Unable to delete namespaces in error state")
			return ctrl.Result{Requeue: true}, err
		}
	}

	r.Log.Info(fmt.Sprintf("'%s' pool status", pool.Name),
		"ready", pool.Status.Ready,
		"creating", pool.Status.Creating,
		"reserved", pool.Status.Reserved)

	quantityOfNamespaces := r.getNamespaceQuantityDelta(pool)

	if quantityOfNamespaces > 0 {
		r.Log.Info(fmt.Sprintf("filling [%s] pool with [%d] namespace(s)", pool.Name, quantityOfNamespaces))
		err := r.increaseReadyNamespacesQueue(ctx, pool, quantityOfNamespaces)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("unable to create more namespaces for [%s] pool.", pool.Name))
		}
	} else if quantityOfNamespaces < 0 {
		r.Log.Info(fmt.Sprintf("excess number of ready namespaces in [%s] pool detected, removing [%d] namespace(s)", pool.Name, (quantityOfNamespaces * -1)))
		err := r.decreaseReadyNamespacesQueue(ctx, pool, quantityOfNamespaces)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("unable to delete excess namespaces for [%s] pool.", pool.Name))
		}
	} else {
		limit := *pool.Spec.SizeLimit
		r.Log.Info(fmt.Sprintf("max number of namespaces for pool [%s] already created [%d]", pool.Name, limit))
	}

	if err := r.Status().Update(ctx, &pool); err != nil {
		r.Log.Error(err, fmt.Sprintf("cannot update [%s] pool status", pool.Name))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespacePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespacePool{}).
		Watches(&source.Kind{Type: &core.Namespace{}},
			handler.EnqueueRequestsFromMapFunc(r.EnqueueNamespace),
		).
		Complete(r)
}

func (r *NamespacePoolReconciler) EnqueueNamespace(a client.Object) []reconcile.Request {
	labels := a.GetLabels()

	if pool, ok := labels["pool"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name: pool,
			},
		}}
	}
	return []reconcile.Request{}

}

func (r *NamespacePoolReconciler) handleErrorNamespaces(ctx context.Context, errNamespaceList []string) error {
	for _, nsName := range errNamespaceList {
		r.Log.Info("deleting namespace", "namespace", nsName)
		err := helpers.DeleteNamespace(ctx, r.Client, nsName)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("error deleting namespace: [%s]", nsName))
			return fmt.Errorf("handleErrorNamespace error: Couldn't delete namespace: [%v]", err)
		}
	}

	return nil
}

func (r *NamespacePoolReconciler) getPoolStatus(ctx context.Context, pool *crd.NamespacePool) ([]string, error) {
	var readyNamespaceCount int
	var creatingNamespaceCount int
	var reservedNamespaceCount int

	nsList := core.NamespaceList{}
	errNamespaceList := []string{}

	labelSelector, _ := labels.Parse(fmt.Sprintf("pool=%s", pool.Name))
	nsListOptions := &client.ListOptions{LabelSelector: labelSelector}
	if err := r.Client.List(ctx, &nsList, nsListOptions); err != nil {
		r.Log.Error(err, "unable to retrieve list of existing ready namespaces")
		return nil, err
	}

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.UID == pool.GetUID() {
				switch ns.Annotations[helpers.ANNOTATION_ENV_STATUS] {
				case helpers.ENV_STATUS_READY:
					readyNamespaceCount++
				case helpers.ENV_STATUS_CREATING:
					creatingNamespaceCount++
				case helpers.ENV_STATUS_ERROR:
					r.Log.Info("prepping for deletion due to error status", "namespace", ns.Name)
					errNamespaceList = append(errNamespaceList, ns.Name)
				}
			} else if owner.Kind == "NamespaceReservation" {
				reservedNamespaceCount++
			}
		}
	}

	pool.Status.Ready = readyNamespaceCount
	pool.Status.Creating = creatingNamespaceCount
	pool.Status.Reserved = reservedNamespaceCount

	return errNamespaceList, nil
}

func calculatePoolNamespaceChanges(sizeLimit int, size int, namespaceReady int, namespacesCreating int, namespaceReserved int) int {
	if sizeLimit-(namespaceReserved+namespaceReady+namespacesCreating) < size-(namespaceReady+namespacesCreating) {
		return sizeLimit - (namespaceReserved + namespaceReady + namespacesCreating)
	}

	return size - (namespaceReady + namespacesCreating)
}

func (r *NamespacePoolReconciler) getNamespaceQuantityDelta(pool crd.NamespacePool) int {
	poolSizeLimit := pool.Spec.SizeLimit
	size := pool.Spec.Size
	namespaceReady := pool.Status.Ready
	namespacesCreating := pool.Status.Creating
	namespaceReserved := pool.Status.Reserved

	if poolSizeLimit == nil {
		return size - (namespaceReady + namespacesCreating)
	}

	sizeLimit := *poolSizeLimit

	return calculatePoolNamespaceChanges(sizeLimit, size, namespaceReady, namespacesCreating, namespaceReserved)

}

func (r *NamespacePoolReconciler) increaseReadyNamespacesQueue(ctx context.Context, pool crd.NamespacePool, increaseSize int) error {
	for i := 0; i < r.getNamespaceQuantityDelta(pool); i++ {
		nsName, err := helpers.CreateNamespace(ctx, r.Client, &pool)
		if err != nil {
			r.Log.Error(err, "error while creating namespace")
			if nsName != "" {
				err := helpers.UpdateAnnotations(ctx, r.Client, nsName, helpers.AnnotationEnvError.ToMap())
				if err != nil {
					r.Log.Error(err, "error while updating annotations on namespace", "namespace", nsName)
					return err
				}
			}

			return err
		}

		r.Log.Info(fmt.Sprintf("successfully created namespace [%s] in [%s] pool", nsName, pool.Name))
	}

	return nil
}

func (r *NamespacePoolReconciler) decreaseReadyNamespacesQueue(ctx context.Context, pool crd.NamespacePool, decreaseSize int) error {
	nsList, err := helpers.GetReadyNamespaces(ctx, r.Client, pool.Name)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("unable to retrieve list of namespaces from '%s' pool", pool.Name))
		return err
	}

	for i := decreaseSize; i < 0; i++ {
		for _, ns := range nsList {
			if ns.Annotations[helpers.ANNOTATION_ENV_STATUS] == helpers.ENV_STATUS_READY && ns.Annotations[helpers.ANNOTATION_RESERVED] == "false" {
				err := helpers.UpdateAnnotations(ctx, r.Client, ns.Name, helpers.AnnotationEnvError.ToMap())
				if err != nil {
					r.Log.Error(err, "error while updating annotations on namespace", "namespace", ns.Name)
					return err
				}

				r.Log.Info(fmt.Sprintf("successfully deleted excess namespace [%s] from [%s] pool", ns.Name, pool.Name))
				break
			}
		}
	}

	return nil
}
