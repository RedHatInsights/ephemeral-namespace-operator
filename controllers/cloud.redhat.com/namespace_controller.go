package controllers

import (
	"context"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
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
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespace,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespace/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespace/finalizers,verbs=update

func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
