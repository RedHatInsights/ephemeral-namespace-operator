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

	status, errNamespaceList, err := r.getPoolStatus(ctx, pool)
	if err != nil {
		r.Log.Error(err, "Unable to get status of owned namespaces")
		return ctrl.Result{}, err
	}

	err = r.handleErrorNamespaces(ctx, errNamespaceList)
	if err != nil {
		r.Log.Error(err, "Unable to delete object.")
		return ctrl.Result{Requeue: true}, err
	}

	r.Log.Info("Pool status", "ready", status["ready"], "creating", status["creating"])

	pool.Status.Ready = status["ready"]
	pool.Status.Creating = status["creating"]

	for i := r.underManaged(pool); i > 0; i-- {
		nsName, err := CreateNamespace(ctx, r.Client, &pool)
		if err != nil {
			r.Log.Error(err, "Error while creating namespace")
			if nsName != "" {
				ns, err := GetNamespace(ctx, r.Client, nsName)
				if err != nil {
					r.Log.Error(err, "Could not retrieve namespace for deletion", "ns-name", nsName)
				} else {
					r.Client.Delete(ctx, &ns)
				}
			}

		} else {
			r.Log.Info("Setting up new namespace", "ns-name", nsName, "pool-type", pool.Name)
			if err := SetupNamespace(ctx, r.Client, pool, nsName); err != nil {
				r.Log.Error(err, "Error while setting up namespace", "ns-name", nsName)
				if err := UpdateAnnotations(ctx, r.Client, map[string]string{"env-status": "error"}, nsName); err != nil {
					r.Log.Error(err, "Error while updating annotations on namespace", "ns-name", nsName)
					// Last resort - if annotations can't be updated attempt manual deletion of namespace
					ns, err := GetNamespace(ctx, r.Client, nsName)
					if err != nil {
						r.Log.Error(err, "Could not retrieve namespace for deletion", "ns-name", nsName)
					} else {
						r.Client.Delete(ctx, &ns)
					}
				}
				continue
			}
			pool.Status.Creating++
		}
	}

	if err := r.Status().Update(ctx, &pool); err != nil {
		r.Log.Error(err, "Cannot update pool status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

//namespaceCreationTimeMetricsontroller with the Manager.
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

		removed, err := CheckForSubscriptionPrometheusOperator(ctx, r.Client, nsName)
		if !removed {
			err := fmt.Errorf("subscription not yet removed from [%s]", nsName)
			return err
		} else if err != nil {
			return err
		}

		_, err = DeletePrometheusOperator(ctx, r.Client, r.Log, nsName)
		if err != nil {
			return fmt.Errorf("error deleting prom operator: %s", err.Error())
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

	status := make(map[string]int)
	status["ready"] = readyNS
	status["creating"] = creatingNS

	return status, errNamespaceList, nil
}

func (r *NamespacePoolReconciler) underManaged(pool crd.NamespacePool) int {
	size := pool.Spec.Size
	ready := pool.Status.Ready
	creating := pool.Status.Creating

	return size - (ready + creating)
}
