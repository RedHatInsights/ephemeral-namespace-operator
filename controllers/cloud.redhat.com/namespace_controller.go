package controllers

import (
	"context"
	"fmt"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type NamespaceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespace,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespace/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespace/finalizers,verbs=update

func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := core.Namespace{}

	err := r.client.Get(ctx, req.NamespacedName, &namespace)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("could not retrieve namespace [%s]: %s", namespace.Name, err)
	}

	if namespace.Annotations[helpers.AnnotationEnvStatus] == helpers.EnvStatusError {
		err = r.client.Delete(ctx, &namespace)
		r.log.Info(fmt.Sprintf("namespace [%s] was in error state and has been deleted", namespace.Name))
		if err != nil {
			r.log.Error(err, "Unable to delete namespaces in error state")
			return ctrl.Result{Requeue: true}, err
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&core.Namespace{}).
		Watches(&source.Kind{Type: &crd.NamespacePool{}},
			handler.EnqueueRequestsFromMapFunc(r.EnqueueNamespace),
		).
		Complete(r)
}

func (r *NamespaceReconciler) EnqueueNamespace(a client.Object) []reconcile.Request {
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
