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
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ClowdenvironmentReconciler reconciles a Clowdenvironment object
type ClowdenvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments/status,verbs=get

func (r *ClowdenvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	env := clowder.ClowdEnvironment{}
	if err := r.Client.Get(ctx, req.NamespacedName, &env); err != nil {
		r.Log.Error(err, "Error retrieving clowdenv", "env-name", env.Name)
		return ctrl.Result{}, err
	}

	r.Log.Info(
		"Reconciling clowdenv",
		"env-name", env.Name,
		"deployments", fmt.Sprintf("%d / %d", env.Status.Deployments.ReadyDeployments, env.Status.Deployments.ManagedDeployments),
	)

	if ready, _ := helpers.VerifyClowdEnvReady(env); ready {
		nsName := env.Spec.TargetNamespace
		r.Log.Info("Clowdenvironment ready", "namespace", nsName)

		if err := helpers.CreateFrontendEnv(ctx, r.Client, nsName, env); err != nil {
			r.Log.Error(err, "Error encountered with frontend environment", "namespace", nsName)
			helpers.UpdateAnnotations(ctx, r.Client, nsName, helpers.AnnotationEnvError.ToMap())
		} else {
			r.Log.Info("Namespace ready", "namespace", nsName)
			helpers.UpdateAnnotations(ctx, r.Client, nsName, helpers.AnnotationEnvReady.ToMap())

			ns, err := helpers.GetNamespace(ctx, r.Client, nsName)
			if err != nil {
				r.Log.Error(err, "Could not retrieve newly created namespace", "namespace", nsName)
			}

			if _, ok := ns.Annotations[helpers.COMPLETION_TIME]; !ok {
				nsCompletionTime := time.Now()
				ns.Annotations[helpers.COMPLETION_TIME] = nsCompletionTime.String()

				if err := r.Client.Update(ctx, &ns); err != nil {
					return ctrl.Result{}, err
				}

				elapsed := nsCompletionTime.Sub(ns.CreationTimestamp.Time)

				averageNamespaceCreationMetrics.With(prometheus.Labels{"pool": ns.Labels["pool"]}).Observe(float64(elapsed.Seconds()))
			}

		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClowdenvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()
	return ctrl.NewControllerManagedBy(mgr).
		For(&clowder.ClowdEnvironment{}).
		WithEventFilter(poolFilter(ctx, r.Client)).
		Complete(r)
}

func poolFilter(ctx context.Context, cl client.Client) predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newObject := e.ObjectNew.(*clowder.ClowdEnvironment)
			return isOwnedByPool(ctx, cl, newObject.Spec.TargetNamespace)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			object := e.Object.(*clowder.ClowdEnvironment)
			return isOwnedByPool(ctx, cl, object.Spec.TargetNamespace)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func isOwnedByPool(ctx context.Context, cl client.Client, nsName string) bool {
	ns, err := helpers.GetNamespace(ctx, cl, nsName)
	if err != nil {
		return false
	}
	for _, owner := range ns.GetOwnerReferences() {
		if owner.Kind == "NamespacePool" {
			return true
		}
	}

	return false
}

func isOwnedBySpecificPool(ctx context.Context, cl client.Client, nsName string, uid types.UID) bool {
	ns, err := helpers.GetNamespace(ctx, cl, nsName)
	if err != nil {
		return false
	}
	for _, owner := range ns.GetOwnerReferences() {
		if owner.Kind == "NamespacePool" && owner.UID == uid {
			return true
		}
	}

	return false
}
