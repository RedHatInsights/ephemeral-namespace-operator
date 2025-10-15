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
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	utils "github.com/RedHatInsights/rhc-osdk-utils/utils"
	core "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var k8sClient client.Client
var testEnv *envtest.Environment
var stopController context.CancelFunc

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t,
		"Controller Suite",
		Label("Ephemeral Namespace Operator"))
}

// routine that will auto-update ClowdEnvironment status during suite test run
func populateClowdEnvStatus(client client.Client) {
	ctx := context.Background()

	for {
		time.Sleep(time.Duration(1 * time.Second))
		clowdEnvs := clowder.ClowdEnvironmentList{}
		err := client.List(ctx, &clowdEnvs)
		if err != nil {
			continue
		}
		for _, env := range clowdEnvs.Items {
			innerEnv := env
			if len(innerEnv.Status.Conditions) == 0 {
				status := clowder.ClowdEnvironmentStatus{
					Conditions: []clusterv1.Condition{
						{
							Type:               clowder.ReconciliationSuccessful,
							Status:             core.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
						{
							Type:               clowder.DeploymentsReady,
							Status:             core.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
					},
				}
				innerEnv.Status = status
				err := client.Status().Update(ctx, &innerEnv)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}
			}
		}
	}
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "static"), // added to the project manually
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sscheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(k8sscheme))
	utilruntime.Must(clowder.AddToScheme(k8sscheme))
	utilruntime.Must(frontend.AddToScheme(k8sscheme))

	err = crd.AddToScheme(k8sscheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: k8sscheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: k8sscheme,
	})
	Expect(err).ToNot(HaveOccurred())

	testPoolSpec := crd.NamespacePoolSpec{
		Size:  2,
		Local: true,
		ClowdEnvironment: clowder.ClowdEnvironmentSpec{
			Providers: clowder.ProvidersConfig{
				Kafka: clowder.KafkaConfig{
					Mode: "operator",
					Cluster: clowder.KafkaClusterConfig{
						Name:      "kafka",
						Namespace: "kafka",
						Replicas:  5,
					},
				},
				Database: clowder.DatabaseConfig{
					Mode: "local",
				},
				Logging: clowder.LoggingConfig{
					Mode: "none",
				},
				ObjectStore: clowder.ObjectStoreConfig{
					Mode: "minio",
				},
				InMemoryDB: clowder.InMemoryDBConfig{
					Mode: "redis",
				},
				Web: clowder.WebConfig{
					Port: int32(8000),
					Mode: "none",
				},
				Metrics: clowder.MetricsConfig{
					Port: int32(9000),
					Path: "/metrics",
					Mode: "none",
				},
				FeatureFlags: clowder.FeatureFlagsConfig{
					Mode: "local",
				},
				Testing: clowder.TestingConfig{
					ConfigAccess:   "environment",
					K8SAccessLevel: "edit",
					Iqe: clowder.IqeConfig{
						ImageBase: "quay.io/cloudservices/iqe-tests",
					},
				},
				AutoScaler: clowder.AutoScalerConfig{
					Mode: "keda",
				},
			},
		},
		LimitRange: core.LimitRange{
			Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{},
			},
		},
		ResourceQuotas: core.ResourceQuotaList{
			Items: []core.ResourceQuota{},
		},
	}

	poller := Poller{
		client:             k8sManager.GetClient(),
		activeReservations: make(map[string]metav1.Time),
		log:                ctrl.Log.WithName("Poller"),
	}

	err = (&NamespacePoolReconciler{
		client: k8sManager.GetClient(),
		scheme: k8sManager.GetScheme(),
		log:    ctrl.Log.WithName("controllers").WithName("NamespacePoolReconciler"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&NamespaceReservationReconciler{
		client: k8sManager.GetClient(),
		scheme: k8sManager.GetScheme(),
		poller: &poller,
		log:    ctrl.Log.WithName("controllers").WithName("NamespaceReconciler"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ClowdenvironmentReconciler{
		client: k8sManager.GetClient(),
		scheme: k8sManager.GetScheme(),
		log:    ctrl.Log.WithName("controllers").WithName("ClowdEnvController"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	stopController = cancel

	defaultPool := &crd.NamespacePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cloud.redhat.com/",
			Kind:       "NamespacePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: testPoolSpec,
	}

	minimalPool := &crd.NamespacePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cloud.redhat.com/",
			Kind:       "NamespacePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "minimal",
		},
		Spec: testPoolSpec,
	}

	limitPool := &crd.NamespacePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cloud.redhat.com/",
			Kind:       "NamespacePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "limit",
		},
		Spec: testPoolSpec,
	}

	limitPool.Spec.SizeLimit = utils.IntPtr(3)

	Expect(k8sClient.Create(ctx, defaultPool)).Should(Succeed())
	Expect(k8sClient.Create(ctx, minimalPool)).Should(Succeed())
	Expect(k8sClient.Create(ctx, limitPool)).Should(Succeed())

	go poller.Poll()

	go populateClowdEnvStatus(k8sManager.GetClient())

	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	stopController()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// Additional tests for Poller functionality
var _ = Describe("Poller", func() {
	var (
		ctx    context.Context
		poller *Poller
		logger logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logf.Log.WithName("test-poller")

		poller = &Poller{
			client:             k8sClient,
			activeReservations: make(map[string]metav1.Time),
			log:                logger,
		}
	})

	Describe("namespaceIsExpired", func() {
		It("should return true for expired timestamp", func() {
			expiredTime := metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
			result := poller.namespaceIsExpired(expiredTime)
			Expect(result).To(BeTrue())
		})

		It("should return false for future timestamp", func() {
			futureTime := metav1.Time{Time: time.Now().Add(1 * time.Hour)}
			result := poller.namespaceIsExpired(futureTime)
			Expect(result).To(BeFalse())
		})

		It("should return false for zero timestamp", func() {
			zeroTime := metav1.Time{}
			result := poller.namespaceIsExpired(zeroTime)
			Expect(result).To(BeFalse())
		})

		It("should return true for timestamp exactly at current time", func() {
			// Use a time slightly in the past to account for execution time
			pastTime := metav1.Time{Time: time.Now().Add(-1 * time.Millisecond)}
			result := poller.namespaceIsExpired(pastTime)
			Expect(result).To(BeTrue())
		})
	})

	Describe("getExistingReservations", func() {
		It("should return all reservations when they exist", func() {
			resList, err := poller.getExistingReservations(ctx)
			Expect(err).ToNot(HaveOccurred())
			// The list should contain reservations from other tests
			Expect(resList).ToNot(BeNil())
		})
	})

	Describe("populateActiveReservations", func() {
		It("should handle empty reservation list", func() {
			// Clear any existing reservations from the map
			for k := range poller.activeReservations {
				delete(poller.activeReservations, k)
			}

			err := poller.populateActiveReservations(ctx)
			Expect(err).ToNot(HaveOccurred())
			// The map might not be empty due to other tests, so just check no error
		})

		It("should only add reservations with 'active' state", func() {
			// Clear any existing reservations from the map
			for k := range poller.activeReservations {
				delete(poller.activeReservations, k)
			}

			err := poller.populateActiveReservations(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Check that all reservations in the map are from active reservations
			for resName := range poller.activeReservations {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: resName}, res)
				if err == nil {
					Expect(res.Status.State).To(Equal("active"))
				}
			}
		})
	})

	Describe("Poll method behavior", func() {
		It("should handle expired reservations in activeReservations map", func() {
			// Add an expired reservation to the active reservations map
			expiredTime := metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
			poller.activeReservations["test-expired-res"] = expiredTime

			// Manually check the expiration logic (simulating what Poll does)
			for k, v := range poller.activeReservations {
				if poller.namespaceIsExpired(v) {
					delete(poller.activeReservations, k)
				}
			}

			// The expired reservation should be removed from the map
			Expect(poller.activeReservations).ToNot(HaveKey("test-expired-res"))
		})

		It("should not remove non-expired reservations", func() {
			// Add a non-expired reservation to the active reservations map
			futureTime := metav1.Time{Time: time.Now().Add(1 * time.Hour)}
			poller.activeReservations["test-active-res"] = futureTime

			// Manually check the expiration logic
			for k, v := range poller.activeReservations {
				if poller.namespaceIsExpired(v) {
					delete(poller.activeReservations, k)
				}
			}

			// The non-expired reservation should remain in the map
			Expect(poller.activeReservations).To(HaveKey("test-active-res"))
		})
	})

	Describe("PollCycle constant", func() {
		It("should have the expected value", func() {
			Expect(PollCycle).To(Equal(time.Duration(10)))
		})
	})
})

// Additional tests for Metrics functionality
var _ = Describe("Metrics", func() {
	Describe("Prometheus Metrics", func() {
		It("should have totalSuccessfulPoolReservationsCountMetrics defined", func() {
			Expect(totalSuccessfulPoolReservationsCountMetrics).ToNot(BeNil())
		})

		It("should have totalFailedPoolReservationsCountMetrics defined", func() {
			Expect(totalFailedPoolReservationsCountMetrics).ToNot(BeNil())
		})

		It("should have averageRequestedDurationMetrics defined", func() {
			Expect(averageRequestedDurationMetrics).ToNot(BeNil())
		})

		It("should have averageNamespaceCreationMetrics defined", func() {
			Expect(averageNamespaceCreationMetrics).ToNot(BeNil())
		})

		It("should have averageReservationToDeploymentMetrics defined", func() {
			Expect(averageReservationToDeploymentMetrics).ToNot(BeNil())
		})

		It("should have activeReservationTotalMetrics defined", func() {
			Expect(activeReservationTotalMetrics).ToNot(BeNil())
		})

		It("should have resQuantityByUserMetrics defined", func() {
			Expect(resQuantityByUserMetrics).ToNot(BeNil())
		})

		It("should have enoVersion defined", func() {
			Expect(enoVersion).ToNot(BeNil())
		})
	})

	Describe("userNamespaceReservationCount map", func() {
		It("should allow adding and retrieving values", func() {
			// Test the map functionality
			userNamespaceReservationCount["test-user"] = 5
			Expect(userNamespaceReservationCount["test-user"]).To(Equal(5))
		})
	})
})
