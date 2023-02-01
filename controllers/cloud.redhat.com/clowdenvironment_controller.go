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
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ClowdenvironmentReconciler reconciles a Clowdenvironment object
type ClowdenvironmentReconciler struct {
	ctx    context.Context
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments/status,verbs=get

func (r *ClowdenvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	env := clowder.ClowdEnvironment{}
	if err := r.client.Get(ctx, req.NamespacedName, &env); err != nil {
		r.log.Error(err, "Error retrieving clowdenv", "env-name", env.Name)
		return ctrl.Result{}, err
	}

	r.log.Info(
		"Reconciling clowdenv",
		"env-name", env.Name,
		"deployments", fmt.Sprintf("%d / %d", env.Status.Deployments.ReadyDeployments, env.Status.Deployments.ManagedDeployments),
	)

	if ready, err := helpers.VerifyClowdEnvReady(env); !ready {
		return ctrl.Result{Requeue: true}, err
	}

	namespacesName := env.Spec.TargetNamespace
	r.log.Info("clowdenvironment ready", "namespace", namespacesName)

	if err := helpers.CreateFrontendEnv(ctx, r.client, namespacesName, env); err != nil {
		r.log.Error(err, "error encountered with frontend environment", "namespace", namespacesName)
		if aerr := helpers.UpdateAnnotations(ctx, r.client, namespacesName, helpers.AnnotationEnvError.ToMap()); aerr != nil {
			return ctrl.Result{Requeue: true}, fmt.Errorf("error setting annotations: %w", aerr)
		}
	}

	r.log.Info("namespace ready", "namespace", namespacesName)
	if err := helpers.UpdateAnnotations(ctx, r.client, namespacesName, helpers.AnnotationEnvReady.ToMap()); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("error setting annotations: %w", err)
	}

	namespace := core.Namespace{}
	err := r.client.Get(ctx, types.NamespacedName{Name: namespacesName}, &namespace)
	if err != nil {
		r.log.Error(err, "could not retrieve newly created namespace", "namespace", namespacesName)
	}

	if _, ok := namespace.Annotations[helpers.CompletionTime]; ok {
		return ctrl.Result{}, nil
	}

	nsCompletionTime := time.Now()
	var AnnotationCompletionTime = helpers.CustomAnnotation{Annotation: helpers.CompletionTime, Value: nsCompletionTime.String()}

	err = helpers.UpdateAnnotations(ctx, r.client, namespace.Name, AnnotationCompletionTime.ToMap())
	if err != nil {
		r.log.Error(err, "could not update annotation with completion time", "namespace", namespacesName)
	}

	if err := r.client.Update(ctx, &namespace); err != nil {
		return ctrl.Result{}, err
	}

	elapsed := nsCompletionTime.Sub(namespace.CreationTimestamp.Time)

	averageNamespaceCreationMetrics.With(prometheus.Labels{"pool": namespace.Labels["pool"]}).Observe(float64(elapsed.Seconds()))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClowdenvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()
	return ctrl.NewControllerManagedBy(mgr).
		For(&clowder.ClowdEnvironment{}).
		WithEventFilter(poolFilter(ctx, r.client)).
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

func isOwnedByPool(ctx context.Context, cl client.Client, namespaceName string) bool {
	namespace := core.Namespace{}
	if err := cl.Get(ctx, types.NamespacedName{Name: namespaceName}, &namespace); err != nil {
		return false
	}

	for _, owner := range namespace.GetOwnerReferences() {
		if owner.Kind == "NamespacePool" {
			return true
		}
	}

	return false
}

func isOwnedBySpecificPool(ctx context.Context, cl client.Client, namespacesName string, uid types.UID) bool {
	namespace := core.Namespace{}
	if err := cl.Get(ctx, types.NamespacedName{Name: namespacesName}, &namespace); err != nil {
		return false
	}

	for _, owner := range namespace.GetOwnerReferences() {
		if owner.Kind == "NamespacePool" && owner.UID == uid {
			return true
		}
	}

	return false
}
