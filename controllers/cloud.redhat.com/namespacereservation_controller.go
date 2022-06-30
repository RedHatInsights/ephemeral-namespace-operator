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

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8serr "k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// NamespaceReservationReconciler reconciles a NamespaceReservation object
type NamespaceReservationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// NamespacePool *NamespacePool
	Poller *Poller
	Log    logr.Logger
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

	if res.Status.Pool == "" {
		if res.Spec.Pool == "" {
			res.Status.Pool = "default"
		} else {
			res.Status.Pool = res.Spec.Pool
		}
	}

	switch res.Status.State {
	case "active":
		r.Log.Info("Reconciling active reservation", "name", res.Name, "namespace", res.Status.Namespace)
		expirationTS, err := getExpirationTime(&res)
		if err != nil {
			r.Log.Error(err, "Could not get expiration time for reservation", "name", res.Name)
			return ctrl.Result{}, err
		}

		res.Status.Expiration = expirationTS
		r.Poller.ActiveReservations[res.Name] = expirationTS

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
		if r.Poller.namespaceIsExpired(expirationTS) {
			err = DeletePrometheusOperator(ctx, r.Client, res.Status.Namespace)
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("Error deleting prometheus.%s", res.Status.Namespace))
				return ctrl.Result{Requeue: true}, err
			}

			if err := r.Client.Delete(ctx, &res); err != nil {
				r.Log.Error(err, "Unable to delete waiting reservation", "res-name", res.Name)
			} else {
				r.Log.Info("Reservation for namespace has expired while waiting. Deleting.", "res-name", res.Name)
			}
			return ctrl.Result{}, nil
		}
		fallthrough // fallthrough to default case to check for ns availability if not expired

	default:
		// if no, requeue and wait for pool to populate
		r.Log.Info("Reconciling reservation", "name", res.Name)
		r.Log.Info(fmt.Sprintf("Checking %s pool for ready namespaces", res.Status.Pool), "name", res.Name)

		expirationTS, err := getExpirationTime(&res)
		if err != nil {
			r.Log.Error(err, "Could not set expiration time on reservation. Deleting", "res-name", res.Name)
			r.Client.Delete(ctx, &res)
			return ctrl.Result{}, err
		}

		nsList, err := GetReadyNamespaces(ctx, r.Client, res.Status.Pool)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Unable to retrieve list of namespaces from '%s' pool", res.Status.Pool), "res-name", res.Name)
			return ctrl.Result{}, err
		}

		if len(nsList) < 1 {
			r.Log.Info(fmt.Sprintf("Requeue to wait for namespace population from '%s' pool", res.Status.Pool), "name", res.Name)
			if res.Status.State == "" {
				res.Status.State = "waiting"
				res.Status.Expiration = expirationTS
				err := r.Status().Update(ctx, &res)
				if err != nil {
					r.Log.Error(err, "Cannot update status", "name", res.Name)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{Requeue: true}, nil
		}

		// Check to see if there's an error with the Get
		readyNsName := nsList[0].Name
		r.Log.Info(fmt.Sprintf("Found namespace in '%s' pool; verifying ready status", res.Status.Pool))

		// Verify that the ClowdEnv has been set up for the requested namespace
		if err := r.verifyClowdEnvForReadyNs(ctx, readyNsName); err != nil {
			r.Log.Error(err, err.Error(), "ns-name", readyNsName)
			errorAnnotation := map[string]string{
				"env-status": "error",
			}
			if err := UpdateAnnotations(ctx, r.Client, errorAnnotation, readyNsName); err != nil {
				r.Log.Error(err, fmt.Sprintf("Unable to update annotations for unready namespace in '%s' pool", res.Status.Pool), "ns-name", readyNsName)
			}
			return ctrl.Result{Requeue: true}, err
		}

		// Resolve the requested namespace and remove it from the pool
		if err := r.reserveNamespace(ctx, readyNsName, &res); err != nil {
			r.Log.Error(err, fmt.Sprintf("Could not reserve namespace from '%s' pool", res.Status.Pool), "ns-name", readyNsName)
			return ctrl.Result{Requeue: true}, err
		}

		// Update reservation status fields
		res.Status.Namespace = readyNsName
		res.Status.Expiration = expirationTS
		res.Status.State = "active"

		r.Poller.ActiveReservations[res.Name] = expirationTS

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

		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespaceReservation{}).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Duration(500*time.Millisecond), time.Duration(60*time.Second)),
		}).
		Complete(r)
}

func (r *NamespaceReservationReconciler) reserveNamespace(ctx context.Context, readyNsName string, res *crd.NamespaceReservation) error {
	nsObject := core.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: readyNsName}, &nsObject)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Could not retrieve namespace from '%s' pool", res.Status.Pool), "name", readyNsName)
		return err
	}

	// Set Owner Reference on the ns we just reserved to ensure
	// the namespace is deleted when the reservation is deleted
	nsObject.SetOwnerReferences([]metav1.OwnerReference{res.MakeOwnerReference()})

	// Set namespace reserved
	// TODO: update bonfire to only ready "status" annotation
	nsObject.Annotations["reserved"] = "true"

	err = r.Client.Update(ctx, &nsObject)
	if err != nil {
		r.Log.Error(err, "Could not update namespace", "ns-name", readyNsName)
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
		return metav1.Time{Time: time.Now()}, err // If these are not error states, we want to return nil
	}

	return metav1.Time{Time: res.CreationTimestamp.Time.Add(duration)}, err // Same here
}

func (r *NamespaceReservationReconciler) verifyClowdEnvForReadyNs(ctx context.Context, readyNsName string) error {
	ready, _, _ := GetClowdEnv(ctx, r.Client, readyNsName)
	if !ready {
		return fmt.Errorf("ClowdEnvironment is not ready for namespace: %s", readyNsName) // No need to wrap the string when fmt does errors for us
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
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     kind,
				Name:     name,
			})
		}

		binding.Subjects = append(binding.Subjects, rbac.Subject{
			Kind:      "ServiceAccount",
			Name:      "ephemeral-bot",
			Namespace: "ephemeral-base",
		})

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
	}
}
