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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
)

const (
	POOL_STATUS_READY    = "ready"
	POOL_STATUS_CREATING = "creating"
)

var (
	statusTypeCount = make(map[string]int)
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
		r.Log.Error(err, "Error retrieving namespace pool")
		return ctrl.Result{}, err
	}

	statusTypeCount, errNamespaceList, err := r.getPoolStatus(ctx, pool)
	if err != nil {
		r.Log.Error(err, "Unable to get status of owned namespaces")
		return ctrl.Result{}, err
	}

	err = r.handleErrorNamespaces(ctx, errNamespaceList)
	if err != nil {
		r.Log.Error(err, "Unable to delete object.")
		return ctrl.Result{Requeue: true}, err
	}

	r.Log.Info(fmt.Sprintf("'%s' pool status", pool.Name), "ready", statusTypeCount[POOL_STATUS_READY], "creating", statusTypeCount[POOL_STATUS_CREATING])

	pool.Status.Ready = statusTypeCount[POOL_STATUS_READY]
	pool.Status.Creating = statusTypeCount[POOL_STATUS_CREATING]

	quantityOfNamespaces := r.checkReadyNamespaceQuantity(pool)

	if quantityOfNamespaces > 0 {
		r.Log.Info(fmt.Sprintf("Filling '%s' pool with %d namespace(s)", pool.Name, quantityOfNamespaces))
		err := r.increaseReadyNamespacesQueue(ctx, pool, quantityOfNamespaces)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("unable to create more namespaces for '%s' pool.", pool.Name))
		}
	} else if quantityOfNamespaces < 0 {
		r.Log.Info(fmt.Sprintf("Excess number of ready namespaces in '%s' pool detected, removing %d namespace(s)", pool.Name, (quantityOfNamespaces * -1)))
		err := r.decreaseReadyNamespacesQueue(ctx, pool, quantityOfNamespaces)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("unable to delete excess namespaces for '%s' pool.", pool.Name))
		}
	}

	if err := r.Status().Update(ctx, &pool); err != nil {
		r.Log.Error(err, "Cannot update pool status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespacePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespacePool{}).
		Watches(&source.Kind{Type: &core.Namespace{}},
			&handler.EnqueueRequestForOwner{IsController: true, OwnerType: &crd.NamespacePool{}}).
		Complete(r)
}

func (r *NamespacePoolReconciler) handleErrorNamespaces(ctx context.Context, errNamespaceList []string) error {
	for _, nsName := range errNamespaceList {
		r.Log.Info("Deleting namespace", "ns-name", nsName)
		err := DeleteNamespace(ctx, r.Client, nsName)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Error deleting namespace: %s", nsName))
			return fmt.Errorf("handleErrorNamespace error: Couldn't delete namespace: %v", err)
		}
	}

	return nil
}

func (r *NamespacePoolReconciler) getPoolStatus(ctx context.Context, pool crd.NamespacePool) (map[string]int, []string, error) {
	nsList := core.NamespaceList{}
	errNamespaceList := []string{}

	labelSelector, _ := labels.Parse("operator-ns=true")
	nsListOptions := &client.ListOptions{LabelSelector: labelSelector}
	if err := r.Client.List(ctx, &nsList, nsListOptions); err != nil {
		r.Log.Error(err, "Unable to retrieve list of existing ready namespaces")
		return nil, nil, err
	}

	var readyNS int
	var creatingNS int

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.UID == pool.GetUID() {
				switch ns.Annotations["env-status"] {
				case "ready":
					readyNS++
				case "creating":
					creatingNS++
				case "error":
					r.Log.Info("Error status for namespace. Prepping for deletion.", "ns-name", ns.Name)
					errNamespaceList = append(errNamespaceList, ns.Name)
				}
			}
		}
	}

	statusTypeCount["ready"] = readyNS
	statusTypeCount["creating"] = creatingNS

	return statusTypeCount, errNamespaceList, nil
}

func (r *NamespacePoolReconciler) checkReadyNamespaceQuantity(pool crd.NamespacePool) int {
	size := pool.Spec.Size
	ready := pool.Status.Ready
	creating := pool.Status.Creating

	return size - (ready + creating)
}

func (r *NamespacePoolReconciler) increaseReadyNamespacesQueue(ctx context.Context, pool crd.NamespacePool, increaseSize int) error {
	for i := 0; i < r.checkReadyNamespaceQuantity(pool); i++ {
		nsName, err := CreateNamespace(ctx, r.Client, &pool)
		if err != nil {
			r.Log.Error(err, "Error while creating namespace")
			if nsName != "" {
				err := UpdateAnnotations(ctx, r.Client, map[string]string{"env-status": "error"}, nsName)
				if err != nil {
					r.Log.Error(err, "Error while updating annotations on namespace", "ns-name", nsName)
					return err
				}
			}

			return err
		}

		r.Log.Info(fmt.Sprintf("successfully created namespace '%s' in '%s' pool", nsName, pool.Name))
	}

	return nil
}

func (r *NamespacePoolReconciler) decreaseReadyNamespacesQueue(ctx context.Context, pool crd.NamespacePool, decreaseSize int) error {
	nsList, err := GetReadyNamespaces(ctx, r.Client, pool.Name)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Unable to retrieve list of namespaces from '%s' pool", pool.Name))
		return err
	}

	for i := 0; i < (decreaseSize * -1); i++ {
		for _, ns := range nsList {
			if ns.Annotations["env-status"] == "ready" && ns.Annotations["reserved"] == "false" {
				err := r.Client.Delete(ctx, &ns)
				if err != nil {
					return err
				}
				r.Log.Info(fmt.Sprintf("successfully deleted excess namespace '%s' from '%s' pool", ns.Name, pool.Name))
			}
		}
	}

	return nil
}
