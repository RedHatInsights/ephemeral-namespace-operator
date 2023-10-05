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

// NamespacePoolReconciler reconciles a NamespacePool object
type NamespacePoolReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacepools/finalizers,verbs=update

func (r *NamespacePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pool := crd.NamespacePool{}

	if err := r.client.Get(ctx, req.NamespacedName, &pool); err != nil {
		r.log.Error(err, fmt.Sprintf("cannot retrieve [%s] pool resource", pool.Name))
		return ctrl.Result{Requeue: true}, err
	}

	errNamespaceList, err := r.getPoolStatus(ctx, &pool)
	if err != nil {
		r.log.Error(err, "unable to get status of owned namespaces")
		return ctrl.Result{Requeue: true}, err
	}

	if len(errNamespaceList) > 0 {
		r.log.Info(fmt.Sprintf("[%d] namespaces in error state are queued for deletion", len(errNamespaceList)))
		err = r.deleteErrorNamespaces(ctx, errNamespaceList)
		if err != nil {
			r.log.Error(err, "Unable to delete namespaces in error state")
			return ctrl.Result{Requeue: true}, err
		}
	}

	r.log.Info(fmt.Sprintf("[%s] pool status", pool.Name),
		"ready", pool.Status.Ready,
		"creating", pool.Status.Creating,
		"reserved", pool.Status.Reserved)

	quantityOfNamespaces := r.getNamespaceQuantityDelta(pool)

	if quantityOfNamespaces > 0 {
		r.log.Info(fmt.Sprintf("filling [%s] pool with [%d] namespace(s)", pool.Name, quantityOfNamespaces))
		err := r.increaseReadyNamespacesQueue(ctx, pool, quantityOfNamespaces)
		if err != nil {
			r.log.Error(err, fmt.Sprintf("unable to create more namespaces for [%s] pool.", pool.Name))
			return ctrl.Result{Requeue: true}, err
		}
	} else if quantityOfNamespaces < 0 {
		r.log.Info(fmt.Sprintf("excess number of ready namespaces in [%s] pool detected, removing [%d] namespace(s)", pool.Name, (quantityOfNamespaces * -1)))
		err := r.decreaseReadyNamespacesQueue(ctx, pool.Name, quantityOfNamespaces)
		if err != nil {
			r.log.Error(err, fmt.Sprintf("unable to delete excess namespaces for [%s] pool.", pool.Name))
			return ctrl.Result{Requeue: true}, err
		}
	}

	if err = r.client.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{Requeue: true}, err
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

func (r *NamespacePoolReconciler) deleteErrorNamespaces(ctx context.Context, errNamespaceList []string) error {
	for _, nsName := range errNamespaceList {
		r.log.Info("deleting namespace", "namespace", nsName)
		err := helpers.DeleteNamespace(ctx, r.client, nsName)
		if err != nil {
			return fmt.Errorf("could not delete namespace in error state: [%w]", err)
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
	if err := r.client.List(ctx, &nsList, nsListOptions); err != nil {
		r.log.Error(err, "unable to retrieve list of existing ready namespaces")
		return nil, err
	}

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.UID == pool.GetUID() {
				switch ns.Annotations[helpers.AnnotationEnvStatus] {
				case helpers.EnvStatusReady:
					readyNamespaceCount++
				case helpers.EnvStatusCreating:
					creatingNamespaceCount++
				case helpers.EnvStatusError:
					r.log.Info("prepping for deletion due to error status", "namespace", ns.Name)
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

func (r *NamespacePoolReconciler) getNamespaceQuantityDelta(pool crd.NamespacePool) int {
	poolSizeLimit := pool.Spec.SizeLimit
	size := pool.Spec.Size
	namespacesReady := pool.Status.Ready
	namespacesCreating := pool.Status.Creating
	namespacesReserved := pool.Status.Reserved
	namespacePoolTotal := namespacesReady + namespacesCreating + namespacesReserved

	namespaceDelta := helpers.CalculateNamespaceQuantityDelta(poolSizeLimit, size, namespacesReady, namespacesCreating, namespacesReserved)

	isAtLimit := false
	if poolSizeLimit != nil {
		isAtLimit = helpers.IsPoolAtLimit(namespacePoolTotal, *poolSizeLimit)
	}

	if namespaceDelta == 0 && isAtLimit {
		r.log.Info(fmt.Sprintf("max number of namespaces for pool [%s] already created", pool.Name), "max namespaces", poolSizeLimit)
	}

	return namespaceDelta
}

func (r *NamespacePoolReconciler) increaseReadyNamespacesQueue(ctx context.Context, pool crd.NamespacePool, increaseSize int) error {
	for i := 0; i < increaseSize; i++ {
		namespaceName, err := helpers.CreateNamespace(ctx, r.client, &pool)
		if err == nil {
			r.log.Info(fmt.Sprintf("successfully created namespace [%s] in [%s] pool", namespaceName, pool.Name))
			continue
		}

		r.log.Error(err, fmt.Sprintf("error while creating namespace [%s]", namespaceName))
		if namespaceName != "" {
			err := helpers.UpdateAnnotations(ctx, r.client, namespaceName, helpers.AnnotationEnvError.ToMap())
			if err != nil {
				r.log.Error(err, "error while updating annotations on namespace", "namespace", namespaceName)
				return err
			}
		}
	}

	return nil
}

func (r *NamespacePoolReconciler) decreaseReadyNamespacesQueue(ctx context.Context, poolName string, decreaseSize int) error {
	namespaceList, err := helpers.GetReadyNamespaces(ctx, r.client, poolName)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("unable to retrieve list of namespaces from [%s] pool", poolName))
		return err
	}

	if len(namespaceList) == 0 {
		r.log.Info(fmt.Sprintf("no ready namespaces to delete for [%s] pool", poolName))
	}

	for i := decreaseSize; i < 0; i++ {
		for _, namespace := range namespaceList {
			if namespace.Annotations[helpers.AnnotationEnvStatus] == helpers.EnvStatusReady && namespace.Annotations[helpers.AnnotationReserved] == "false" {
				err := helpers.UpdateAnnotations(ctx, r.client, namespace.Name, helpers.AnnotationEnvError.ToMap())
				if err != nil {
					r.log.Error(err, "error while updating annotations on namespace", "namespace", namespace.Name)
					return err
				}

				r.log.Info(fmt.Sprintf("successfully deleted excess namespace [%s] from [%s] pool", namespace.Name, poolName))
				break
			}
		}
	}

	return nil
}
