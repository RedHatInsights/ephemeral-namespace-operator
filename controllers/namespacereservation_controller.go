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
	"k8s.io/apimachinery/pkg/runtime"
	"math/rand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	// apps "k8s.io/api/apps/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// k8serr "k8s.io/apimachinery/pkg/api/errors"
	// "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	// "k8s.io/apimachinery/pkg/types"
	// "k8s.io/client-go/tools/record"
	// "k8s.io/client-go/util/workqueue"
)

// NamespaceReservationReconciler reconciles a NamespaceReservation object
type NamespaceReservationReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	OnDeckNamespaces []core.Namespace
}

//+kubebuilder:rbac:groups=cloud.redhat.com.cloud.redhat.com,resources=namespacereservations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com.cloud.redhat.com,resources=namespacereservations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com.cloud.redhat.com,resources=namespacereservations/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;events;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings;roles,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceReservation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *NamespaceReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Fetch the reservation
	// Is there already a namespace assigned?
	// if no, assign namespace from on-deck pool and create new on-deck ns
	// update ready field
	// update expiration timestamp (creation timestamp + duration)

	return ctrl.Result{}, nil
}

func (r *NamespaceReservationReconciler) createOnDeckNamespace(ctx context.Context, name string) error {
	// Create namespace
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s-%s", name, strings.ToLower(randString(6)))
	err := r.Client.Create(ctx, &ns)

	if err != nil {
		return err
	}

	// Create ClowdEnvironment
	// Assign permissions: clowder-edit and edit
	// TODO: hard-coded list of users for now, but will want to do graphql queries later
	// Copy secrets

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespaceReservation{}).
		Watches(
			&source.Kind{Type: &core.Namespace{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsForObject),
		).
		Complete(r)
}

func (r *NamespaceReservationReconciler) enqueueRequestsForObject(a client.Object) []reconcile.Request {
	return []reconcile.Request{}
}

const rCharSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randString(n int) string {
	b := make([]byte, n)

	for i := range b {
		b[i] = rCharSet[rand.Intn(len(rCharSet))]
	}

	return string(b)
}
