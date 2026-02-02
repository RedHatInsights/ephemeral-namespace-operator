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

// Package controllers implements Kubernetes controllers for managing ephemeral namespace pools
package controllers

import (
	"context"
	"fmt"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ClowdenvironmentReconciler reconciles a Clowdenvironment object
type ClowdenvironmentReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments/status,verbs=get

// Reconcile reconciles ClowdEnvironment resources within ephemeral namespaces and sets up FrontendEnvironments
func (r *ClowdenvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("rid", utils.RandString(5))
	ctx = context.WithValue(ctx, helpers.ContextKey("log"), &log)

	env := clowder.ClowdEnvironment{}
	if err := r.client.Get(ctx, req.NamespacedName, &env); err != nil {
		if k8serr.IsNotFound(err) {
			// ClowdEnvironment was deleted, nothing to reconcile
			log.Info("ClowdEnvironment not found, likely deleted", "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}

		r.log.Error(err, "there was an issue retrieving the clowdenvironment", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{Requeue: true}, err
	}

	log.Info(
		"Reconciling clowdenv",
		"env-name", env.Name,
		"deployments", fmt.Sprintf("%d / %d", env.Status.Deployments.ReadyDeployments, env.Status.Deployments.ManagedDeployments),
	)

	if ready := helpers.VerifyClowdEnvReady(env); !ready {
		return ctrl.Result{Requeue: true}, nil
	}

	namespaceName := env.Spec.TargetNamespace
	log.Info("clowdenvironment ready", "namespace", namespaceName)

	if err := helpers.CreateFrontendEnv(ctx, r.client, namespaceName, env); err != nil {
		r.log.Error(err, "error encountered with frontend environment", "namespace", namespaceName)
		if aerr := helpers.UpdateAnnotations(ctx, r.client, namespaceName, helpers.AnnotationEnvError.ToMap()); aerr != nil {
			return ctrl.Result{Requeue: true}, fmt.Errorf("error setting annotations: %w", aerr)
		}
	}

	log.Info("namespace ready", "namespace", namespaceName)
	if err := helpers.UpdateAnnotations(ctx, r.client, namespaceName, helpers.AnnotationEnvReady.ToMap()); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("error setting annotations: %w", err)
	}

	namespace, err := helpers.GetNamespace(ctx, r.client, namespaceName)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("could not retrieve updated namespace %s: %w", namespaceName, err)
	}

	if _, ok := namespace.Annotations[helpers.CompletionTime]; ok {
		return ctrl.Result{}, nil
	}

	nsCompletionTime := time.Now()
	var AnnotationCompletionTime = helpers.CustomAnnotation{Annotation: helpers.CompletionTime, Value: nsCompletionTime.String()}

	err = helpers.UpdateAnnotations(ctx, r.client, namespace.Name, AnnotationCompletionTime.ToMap())
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("could not retrieve updated namespace [%s] after updating annotations: %w", namespaceName, err)
	}

	if err = r.client.Update(ctx, &namespace); err != nil {
		return ctrl.Result{Requeue: true}, err
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
		DeleteFunc: func(_ event.DeleteEvent) bool {
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
