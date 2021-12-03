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
	"errors"

	"fmt"
	"math/rand"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	//"sigs.k8s.io/controller-runtime/pkg/handler"
	// "sigs.k8s.io/controller-runtime/pkg/reconcile"
	// "sigs.k8s.io/controller-runtime/pkg/source"
	// apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	// "k8s.io/client-go/tools/record"
	// "k8s.io/client-go/util/workqueue"
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
//+kubebuilder:rbac:groups="",resources=secrets;events;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="project.openshift.io",resources=projects;projectrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings;roles,verbs=get;list;watch;create;update;patch;delete

func (r *NamespaceReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Determine flow of reconciliations
	// TODO: Determine actions for enqueue on ns events
	// TODO: Determine actions for an unready ns

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

	if res.Status.Namespace != "" {
		r.Log.Info("Reconciling existing NamespaceReservation", "name", res.Name)

		if res.Status.State == "expired" {
			r.Log.Info("Cannot extend expired reservation", "name", res.Name)
			return ctrl.Result{}, nil
		}

		expirationTS, err := getExpirationTime(res.Status.Expiration, &res)
		if err != nil {
			r.Log.Error(err, "Could not set expiration time on reservation", "name", res.Name)
			return ctrl.Result{}, err
		}

		res.Status.Expiration = expirationTS
		r.NamespacePool.ActiveReservations[res.Name] = expirationTS

		if err := r.Status().Update(ctx, &res); err != nil {
			r.Log.Error(err, "Cannot update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	} else {
		r.Log.Info("Reconciling newly created NamespaceReservation", "name", req.Name)

		// if no, requeue and wait for pool to populate
		r.Log.Info("Checking pool for ready namespaces")
		if r.NamespacePool.Len() < 1 {
			r.Log.Info("Requeue to wait for namespace pool population")
			if res.Status.State == "" {
				res.Status.State = "waiting"
				res.Status.Namespace = ""
				res.Status.Expiration, _ = getExpirationTime(res.ObjectMeta.CreationTimestamp, &res)
				err := r.Status().Update(ctx, &res)
				if err != nil {
					r.Log.Error(err, "Cannot update status")
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
		creationTS := res.ObjectMeta.CreationTimestamp
		expirationTS, err := getExpirationTime(creationTS, &res)
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

func eventFilter(log logr.Logger) predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObject := e.ObjectOld.(*crd.NamespaceReservation)
			newObject := e.ObjectNew.(*crd.NamespaceReservation)

			if oldObject.Status != newObject.Status {
				return false
			}

			if oldObject.Annotations["extensionId"] == newObject.Annotations["extensionId"] {
				return false
			}

			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			object := e.Object.(*crd.NamespaceReservation)
			if object.Status.Namespace != "" {
				return false
			}
			return true
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespaceReservation{}).
		WithEventFilter(eventFilter(r.Log)).
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

func getExpirationTime(start metav1.Time, res *crd.NamespaceReservation) (metav1.Time, error) {
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

	return metav1.Time{Time: start.Add(duration)}, err
}

func (r *NamespaceReservationReconciler) verifyClowdEnvForReadyNs(ctx context.Context, readyNsName string) error {
	ns := core.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: readyNsName}, &ns)
	if err != nil {
		return err
	}

	ready, _ := r.NamespacePool.VerifyClowdEnv(ctx, r.Client, ns)
	if !ready {
		return errors.New(fmt.Sprintf("ClowdEnvironment is not ready for namespace: %s", readyNsName))
	}

	return nil
}

func (r *NamespaceReservationReconciler) addRoleBindings(ctx context.Context, ns *core.Namespace, client client.Client) error {

	// Assign permissions: clowder-edit and edit
	// TODO: hard-coded list of users for now, but will want to do graphql queries later
	roleNames := []string{"edit"}

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

		binding.SetName(roleName)
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

func hardCodedEnvSpec() clowder.ClowdEnvironmentSpec {
	return clowder.ClowdEnvironmentSpec{
		ResourceDefaults: core.ResourceRequirements{
			Limits: core.ResourceList{
				core.ResourceCPU:    resource.MustParse("300m"),
				core.ResourceMemory: resource.MustParse("256Mi"),
			},
			Requests: core.ResourceList{
				core.ResourceCPU:    resource.MustParse("30m"),
				core.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		Providers: clowder.ProvidersConfig{
			Database:   clowder.DatabaseConfig{Mode: "local"},
			InMemoryDB: clowder.InMemoryDBConfig{Mode: "redis"},
			PullSecrets: []clowder.NamespacedName{{
				Namespace: "ephemeral-base",
				Name:      "quay-cloudservices-pull",
			}},
			FeatureFlags: clowder.FeatureFlagsConfig{Mode: "local"},
			Metrics: clowder.MetricsConfig{
				Port:       9000,
				Path:       "/metrics",
				Prometheus: clowder.PrometheusConfig{Deploy: true},
				Mode:       "operator",
			},
			Logging:     clowder.LoggingConfig{Mode: "none"},
			ObjectStore: clowder.ObjectStoreConfig{Mode: "minio"},
			Web: clowder.WebConfig{
				Port:        8000,
				PrivatePort: 10000,
				Mode:        "operator",
			},
			Kafka: clowder.KafkaConfig{
				Mode:                "operator",
				EnableLegacyStrimzi: true,
				Cluster:             clowder.KafkaClusterConfig{Version: "2.7.0"},
				Connect: clowder.KafkaConnectClusterConfig{
					Version: "2.7.0",
					Image:   "quay.io/cloudservices/xjoin-kafka-connect-strimzi:182ab8b",
				},
			},
			Testing: clowder.TestingConfig{
				K8SAccessLevel: "edit",
				ConfigAccess:   "environment",
				Iqe: clowder.IqeConfig{
					VaultSecretRef: clowder.NamespacedName{
						Namespace: "ephemeral-base",
						Name:      "iqe-vault",
					},
					ImageBase: "quay.io/cloudservices/iqe-tests",
					Resources: core.ResourceRequirements{
						Limits: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("1"),
							core.ResourceMemory: resource.MustParse("2Gi"),
						},
						Requests: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("200m"),
							core.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
}

func hardCodedUserList() map[string]string {
	return map[string]string{
		"ephemeral-users": "Group",
		"system:serviceaccount:ephemeral-base:ephemeral-bot": "User",
	}
}
