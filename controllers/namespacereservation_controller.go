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
	"math/rand"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8serr "k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NamespaceReservationReconciler reconciles a NamespaceReservation object
type NamespaceReservationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	NamespacePool *NamespacePool
	Log           logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacereservations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacereservations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=namespacereservations/finalizers,verbs=update
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=frontendenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets;events;namespaces;limitranges;resourcequotas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="project.openshift.io",resources=projects;projectrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="config.openshift.io",resources=ingresses,verbs=get;list;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings;roles,verbs=get;list;watch;create;update;patch;delete

func (r *NamespaceReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the reservation
	res := crd.NamespaceReservation{}
	if err := r.Client.Get(ctx, req.NamespacedName, &res); err != nil {
		if k8serr.IsNotFound(err) {
			// Must have been deleted
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "Reservation Not Found")
		return ctrl.Result{}, err
	}

	switch res.Status.State {
	case "expired":
		return ctrl.Result{}, nil

	case "active":
		r.Log.Info("Reconciling active reservation", "name", res.Name, "namespace", res.Status.Namespace)
		expirationTS, err := getExpirationTime(&res)
		if err != nil {
			r.Log.Error(err, "Could not get expiration time for reservation", "name", res.Name)
			return ctrl.Result{}, err
		}

		res.Status.Expiration = expirationTS
		r.NamespacePool.ActiveReservations[res.Name] = expirationTS

		if err := r.Status().Update(ctx, &res); err != nil {
			r.Log.Error(err, "Cannot update reservation status", "name", res.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case "waiting":
		r.Log.Info("Reconciling waiting reservation", "name", res.Name)
		expirationTS, err := getExpirationTime(&res)
		if err != nil {
			r.Log.Error(err, "Could not get expiration time for reservation", "name", res.Name)
			return ctrl.Result{}, err
		}
		if r.NamespacePool.namespaceIsExpired(expirationTS) {
			res.Status.State = "expired"
			err := r.Status().Update(ctx, &res)
			if err != nil {
				r.Log.Error(err, "Cannot update reservation status", "name", res.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		fallthrough // fallthrough to default case to check for ns availability if not expired

	default:
		// if no, requeue and wait for pool to populate
		r.Log.Info("Reconciling reservation", "name", res.Name)
		r.Log.Info("Checking pool for ready namespaces", "name", res.Name)
		if r.NamespacePool.Len() < 1 {
			r.Log.Info("Requeue to wait for namespace pool population", "name", res.Name)
			if res.Status.State == "" {
				res.Status.State = "waiting"
				res.Status.Expiration, _ = getExpirationTime(&res)
				err := r.Status().Update(ctx, &res)
				if err != nil {
					r.Log.Error(err, "Cannot update status", "name", res.Name)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{Requeue: true}, nil
		}

		// Check to see if there's an error with the Get
		readyNsName := r.NamespacePool.GetOnDeckNs()
		r.Log.Info("Found namespace in pool; checking for ready status")

		// Verify that the ClowdEnv has been set up for the requested namespace
		if err := r.verifyClowdEnvForReadyNs(ctx, readyNsName); err != nil {
			r.Log.Error(err, err.Error(), "ns-name", readyNsName)
			r.NamespacePool.CycleFrontToBack()
			return ctrl.Result{Requeue: true}, err
		}

		// Resolve the requested namespace and remove it from the pool
		if err := r.reserveNamespace(ctx, readyNsName, &res); err != nil {
			r.Log.Error(err, "Could not reserve namespace", "ns-name", readyNsName)
			return ctrl.Result{Requeue: true}, err
		}

		// update expiration timestamp (creation timestamp + duration)
		expirationTS, err := getExpirationTime(&res)
		if err != nil {
			r.Log.Error(err, "Could not set expiration time on reservation")
			return ctrl.Result{}, err
		}

		// Update reservation status fields
		res.Status.Namespace = readyNsName
		res.Status.Expiration = expirationTS
		res.Status.State = "active"

		r.NamespacePool.ActiveReservations[res.Name] = expirationTS

		r.Log.Info("Updating NamespaceReservation status")
		r.Log.Info("Reservation details",
			"res-name", res.Name,
			"res-uuid", res.ObjectMeta.UID,
			"created", res.ObjectMeta.CreationTimestamp,
			"spec", res.Spec,
			"status", res.Status,
		)
		if err := r.Status().Update(ctx, &res); err != nil {
			r.Log.Error(err, "Cannot update status")
			return ctrl.Result{}, err
		}

		// Creating a new namespace takes a while
		// The reconciler should not wait to consume new events while
		// creating new namespaces
		go func() {
			if err := r.NamespacePool.CreateOnDeckNamespace(ctx, r.Client); err != nil {
				r.Log.Error(err, "Cannot create replacement namespace")
			}
		}()

		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespaceReservation{}).
		Complete(r)
}

func (r *NamespaceReservationReconciler) reserveNamespace(ctx context.Context, readyNsName string, res *crd.NamespaceReservation) error {
	nsObject := core.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: readyNsName}, &nsObject)
	if err != nil {
		r.Log.Error(err, "Could not retrieve namespace", "name", readyNsName)
		return err
	}

	// Set Owner Reference on the ns we just reserved to ensure
	// the namespace is deleted when the reservation is deleted
	nsObject.SetOwnerReferences([]metav1.OwnerReference{res.MakeOwnerReference()})

	// Set namespace reserved
	nsObject.Annotations["reserved"] = "true"

	err = r.Client.Update(ctx, &nsObject)
	if err != nil {
		r.Log.Error(err, "Could not update namespace", "ns-name", readyNsName)
		return err
	}

	// Remove the namespace from the pool
	if err := r.NamespacePool.CheckoutNs(readyNsName); err != nil {
		r.Log.Error(err, "Could not checkout namespace", "ns-name", readyNsName)
		return err
	}

	// Add rolebinding to the namespace only after it has been owned by the CRD.
	// We need to skip this on minikube
	if err := r.addRoleBindings(ctx, &nsObject, r.Client); err != nil {
		r.Log.Error(err, "Could not apply rolebindings for namespace", "ns-name", readyNsName)
		return err
	}

	return nil
}

func getExpirationTime(res *crd.NamespaceReservation) (metav1.Time, error) {
	var duration time.Duration
	var err error
	if res.Spec.Duration != nil {
		duration, err = time.ParseDuration(*res.Spec.Duration)
	} else {
		// Defaults to 1 hour if not specified in spec
		duration, err = time.ParseDuration("1h")
	}
	if err != nil {
		return metav1.Time{}, err
	}

	if duration == 0 {
		return metav1.Time{Time: time.Now()}, err
	}

	return metav1.Time{Time: res.CreationTimestamp.Time.Add(duration)}, err
}

func (r *NamespaceReservationReconciler) verifyClowdEnvForReadyNs(ctx context.Context, readyNsName string) error {
	ns := core.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: readyNsName}, &ns)
	if err != nil {
		return err
	}

	ready, _, _ := r.NamespacePool.GetClowdEnv(ctx, r.Client, ns)
	if !ready {
		return fmt.Errorf("ClowdEnvironment is not ready for namespace: %s", readyNsName)
	}

	return nil
}

func (r *NamespaceReservationReconciler) addRoleBindings(ctx context.Context, ns *core.Namespace, client client.Client) error {
	// TODO: hard-coded list of users for now, but will want to do graphql queries later
	roleNames := []string{"admin"}

	for _, roleName := range roleNames {
		binding := rbac.RoleBinding{
			RoleRef: rbac.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     roleName,
			},
			Subjects: []rbac.Subject{},
		}

		for name, kind := range hardCodedUserList() {
			r.Log.Info(fmt.Sprintf("Creating rolebinding %s for %s: %s", roleName, kind, name), "ns-name", ns.Name)
			binding.Subjects = append(binding.Subjects, rbac.Subject{
				APIGroup:  "rbac.authorization.k8s.io",
				Kind:      kind,
				Name:      name,
				Namespace: ns.Name,
			})
		}

		binding.SetName(fmt.Sprintf("%s-%s", ns.Name, roleName))
		binding.SetNamespace(ns.Name)

		if err := client.Create(ctx, &binding); err != nil {
			return err
		}
	}
	return nil
}

const rCharSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randString(n int) string {
	b := make([]byte, n)

	for i := range b {
		b[i] = rCharSet[rand.Intn(len(rCharSet))]
	}

	return string(b)
}

func hardCodedUserList() map[string]string {
	return map[string]string{
		"ephemeral-users":      "Group",
		"system:authenticated": "Group",
		"system:serviceaccount:ephemeral-base:ephemeral-bot": "ServiceAccount",
	}
}
