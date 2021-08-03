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

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

//+kubebuilder:rbac:groups=cloud.redhat.com.cloud.redhat.com,resources=namespacereservations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com.cloud.redhat.com,resources=namespacereservations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com.cloud.redhat.com,resources=namespacereservations/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;events;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings;roles,verbs=get;list;watch;create;update;patch;delete
func (r *NamespaceReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Update status of namespaces to show reserved
	// TODO: Only have ready envs in pool?
	// TODO: Determine flow of reconciliations
	// TODO: Determine actions of first time spin up
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
	fmt.Printf("%+v", res)
	// TODO: Figure when/why the reconciler is called on already created CRDs
	if res.Status.Namespace != "" {
		// TODO: add support for CRD updates
		return ctrl.Result{}, nil

	} else {
		r.Log.Info("Reconciling for NamespaceReservation")
		output := fmt.Sprintf("Name: %s Namepsace: %s", req.Name, req.Namespace)
		r.Log.Info(output)
		// if no, assign namespace from on-deck pool and create new on-deck ns
		r.Log.Info("Checking pool for ready namespaces")
		if r.NamespacePool.Len() < 1 {
			r.Log.Info("Requeue to wait for namespace pool population")
			return ctrl.Result{Requeue: true}, nil
		}

		// Is there already a namespace assigned?
		r.Log.Info("Checking Reservation status")

		// Check to see if there's an error with the Get
		readyNsName := r.NamespacePool.GetOnDeckNs()
		r.Log.Info("Found namespace in pool; checking for ready status")

		env := clowder.ClowdEnvironment{}
		envErr := r.Client.Get(ctx, types.NamespacedName{
			Name:      readyNsName,
			Namespace: readyNsName,
		}, &env)
		if envErr != nil {
			// Don't requeue, the events in the ns will requeue
			return ctrl.Result{}, envErr
		}

		if !env.IsReady() {
			str := fmt.Sprintf("Namespace %s not yet ready, requeue\n", readyNsName)
			r.Log.Info(str)
			return ctrl.Result{Requeue: true}, nil
		}

		// nsName := metav1.ObjectMeta{
		// 	Name:      req.Name,
		// 	Namespace: req.Namespace,
		// }
		// res.ObjectMeta = nsName
		fmt.Printf("%+v", res)
		if err := r.Client.Update(ctx, &res); err != nil {
			r.Log.Error(err, "cannot create NamespaceReservation")
			return ctrl.Result{}, err
		}

		r.Log.Info("Resolve new NS")
		nsObject := core.Namespace{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: readyNsName}, &nsObject)
		if err != nil {
			if k8serr.IsNotFound(err) {
				// Must have been deleted
				r.Log.Info("Namespace Not Found")
				return ctrl.Result{}, nil
			}
			r.Log.Error(err, "Namespace Not Found")
			return ctrl.Result{}, err
		}

		r.Log.Info("Set ownerrefs")
		// Set Owner Reference on the ns we just reserved
		nsObject.SetOwnerReferences([]metav1.OwnerReference{res.MakeOwnerReference()})
		fmt.Println("NS Owner References: ", nsObject.OwnerReferences)

		err = r.Client.Update(ctx, &nsObject)
		if err != nil {
			fmt.Printf("Could not update namespace ownerReference")
		}

		r.Log.Info("Checkout new NS")
		// Remove the namespace from the pool
		if err := r.NamespacePool.CheckoutNs(readyNsName); err != nil {
			fmt.Printf("Cannot checkout ns")
		}

		// update expiration timestamp (creation timestamp + duration)
		creationTS := res.ObjectMeta.CreationTimestamp

		// TODO: Add support for float hours (2.3) or 1h, 30m (like cpu requests)
		var duration time.Duration
		if res.Spec.Duration != nil {
			duration = time.Duration(*res.Spec.Duration)
		} else {
			// Defaults to 1 hour if not specified in spec
			duration = time.Duration(1)
		}

		r.Log.Info("Add Expiraiton date")
		expirationTS := creationTS.Add(duration * time.Hour)

		// update ready field
		r.Log.Info("Set CRD Status")
		//res.Name = readyNsName
		// res.Status = crd.NamespaceReservationStatus{
		// 	Namespace:  readyNsName,
		// 	Ready:      true,
		// 	Expiration: metav1.Time{Time: expirationTS},
		// }
		res.Status.Namespace = readyNsName
		res.Status.Ready = true
		res.Status.Expiration = metav1.Time{Time: expirationTS}

		r.Log.Info("Add rolebinding to NS")
		// Add rolebinding to the namespace only after it has been owned by the CRD.
		if err := addRoleBindings(ctx, &nsObject, r.Client); err != nil {
			r.Log.Error(err, "cannot apply RoleBindings")
			return ctrl.Result{}, err
		}

		r.Log.Info("Create Update status")
		fmt.Printf("%+v", res)
		if err := r.Status().Update(ctx, &res); err != nil {
			r.Log.Error(err, "Cannot update status")
			return ctrl.Result{}, err
		}

		// r.Log.Info("Create Replacement NS")
		// // Create a replacement NS for the one we just took from the pool
		// if err := r.NamespacePool.CreateOnDeckNamespace(ctx, r.Client); err != nil {
		// 	r.Log.Error(err, "cannot create replacement ns")
		// }

		return ctrl.Result{}, nil
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.NamespaceReservation{}).
		// Watches(
		// 	&source.Kind{Type: &core.Namespace{}},
		// 	handler.EnqueueRequestsFromMapFunc(r.enqueueNamespaceEvent),
		// ).
		Complete(r)
}

// func (r *NamespaceReservationReconciler) enqueueNamespaceEvent(a client.Object) []reconcile.Request {
// 	// TODO: FILTERSSSSSS
// 	reqs := []reconcile.Request{}
// r.Log.Info("Enqueued ns event")
// // TODO: Determine what needs to be handled here
// ctx := context.Background()
// obj := types.NamespacedName{
// 	Name:      a.GetName(),
// 	Namespace: a.GetNamespace(),
// }
// ns := core.Namespace{}
// err := r.Client.Get(ctx, obj, &ns)
// if err != nil {
// 	if k8serr.IsNotFound(err) {
// 		// Must have been deleted
// 		return reqs
// 	}
// 	r.Log.Error(err, "Failed to fetch Namespace")
// 	return nil
// }

// res := crd.NamespaceReservation{}
// err = r.Client.Get(ctx, obj, &res)
// if err != nil {
// 	if k8serr.IsNotFound(err) {
// 		// Must have been deleted
// 		return reqs
// 	}
// 	r.Log.Info("No reservation for Namespace")
// 	return nil
// } else {
// 	reqs = append(reqs, reconcile.Request{
// 		NamespacedName: types.NamespacedName{
// 			Name:      res.Name,
// 			Namespace: res.Namespace,
// 		},
// 	})

// }

// return reqs
// }

func addRoleBindings(ctx context.Context, ns *core.Namespace, client client.Client) error {

	// Assign permissions: clowder-edit and edit
	// TODO: hard-coded list of users for now, but will want to do graphql queries later
	roleNames := []string{"clowder-edit", "edit"}

	for _, roleName := range roleNames {
		binding := rbac.RoleBinding{
			RoleRef: rbac.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     roleName,
			},
			Subjects: []rbac.Subject{},
		}

		for _, user := range hardCodedUserList() {
			binding.Subjects = append(binding.Subjects, rbac.Subject{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     user,
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
				Cluster:             clowder.KafkaClusterConfig{Version: "2.6.0"},
				Connect: clowder.KafkaConnectClusterConfig{
					Version: "2.6.0",
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
							core.ResourceMemory: resource.MustParse("1Gi"),
						},
						Requests: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("200m"),
							core.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	}
}

func hardCodedUserList() []string {
	return []string{
		"kylape",
		"BlakeHolifield",
		"Jason-RH",
	}
}
