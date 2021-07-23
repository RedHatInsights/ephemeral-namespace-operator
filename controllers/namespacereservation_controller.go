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

	"container/list"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/resource"
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

var nsPool = list.List{}

func Poll(client client.Client) error {
	ctx := context.Background()

	for {
		// Check for expired reservations
		// Ensure pool is desired size
		for nsPool.Len() < 5 {
			if err := createOnDeckNamespace(ctx, client); err != nil {
				return err
			}
		}
		time.Sleep(time.Duration(10 * time.Second))
	}
}

func createOnDeckNamespace(ctx context.Context, cl client.Client) error {
	// Create namespace
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))
	err := cl.Create(ctx, &ns)

	if err != nil {
		return err
	}

	// Create ClowdEnvironment

	env := clowder.ClowdEnvironment{Spec: hardCodedEnvSpec()}
	env.SetName(ns.Name)
	env.Spec.TargetNamespace = ns.Name

	cl.Create(ctx, &env)

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

		cl.Create(ctx, &binding)
	}

	// Copy secrets

	secrets := core.SecretList{}
	err = cl.List(ctx, &secrets, client.InNamespace("ephemeral-base"))

	if err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		src := types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}

		dst := types.NamespacedName{
			Name:      secret.Name,
			Namespace: ns.Name,
		}

		utils.CopySecret(ctx, cl, src, dst)
	}

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
	}
}
