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

	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
)

// PoolReconciler reconciles a Pool object
type PoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config OperatorConfig
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=pools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=pools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=pools/finalizers,verbs=update

func (r *PoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pool := crd.Pool{}
	if err := r.Client.Get(ctx, req.NamespacedName, &pool); err != nil {
		r.Log.Error(err, "Error retrieving namespace pool")
		return ctrl.Result{}, err
	}

	status, err := r.getPoolStatus(ctx, pool)
	if err != nil {
		r.Log.Error(err, "Unable to get status of owned namespaces")
		return ctrl.Result{}, err
	}

	pool.Status.Ready = status["ready"]
	pool.Status.Creating = status["creating"]

	r.Log.Info("Namespace Pool Info", "pool", pool)

	for i := r.underManaged(pool); i > 0; i-- {
		ns, err := CreateNamespace(ctx, r.Client, &pool, r.Config.PoolConfig.Local)
		if err != nil {
			r.Log.Error(err, "Error while creating namespace")
			if ns != nil {
				r.Client.Delete(ctx, ns)
			}
		}
		go func() {
			SetupNamespace(ctx, r.Client, r.Config, r.Log, ns.Name)
			// manual requeue after namespace setup complete?
		}()
	}

	if err := r.Status().Update(ctx, &pool); err != nil {
		r.Log.Error(err, "Cannot update pool status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.Pool{}).
		Watches(&source.Kind{Type: &core.Namespace{}}, &handler.EnqueueRequestForOwner{IsController: true, OwnerType: &crd.Pool{}}).
		Complete(r)
}

func (r *PoolReconciler) getPoolStatus(ctx context.Context, pool crd.Pool) (map[string]int, error) {
	nsList := core.NamespaceList{}
	if err := r.Client.List(ctx, &nsList); err != nil {
		r.Log.Error(err, "Unable to retrieve list of existing ready namespaces")
		return nil, err
	}

	var readyNS int
	var creatingNS int

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.UID == pool.GetUID() {
				switch ns.Annotations["status"] {
				case "ready":
					readyNS++
				case "creating":
					creatingNS++
				case "error":
					r.Log.Info("Error status for namespace. Deleting", "ns-name", ns)
					r.Client.Delete(ctx, &ns)
				}
			}
		}
	}

	status := make(map[string]int)
	status["ready"] = readyNS
	status["creating"] = creatingNS

	return status, nil
}

func (r *PoolReconciler) underManaged(pool crd.Pool) int {
	size := pool.Spec.Size
	ready := pool.Status.Ready
	creating := pool.Status.Creating

	return size - (ready + creating)
}
