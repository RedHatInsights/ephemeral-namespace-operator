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
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	utils "github.com/RedHatInsights/rhc-osdk-utils/utils"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
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

	// Create a valid ClowdEnvironment spec for testing
	testClowdEnvSpec := clowder.ClowdEnvironmentSpec{
		Providers: clowder.ProvidersConfig{
			Web: clowder.WebConfig{
				Mode: "none",
			},
			Database: clowder.DatabaseConfig{
				Mode: "none",
			},
			Logging: clowder.LoggingConfig{
				Mode: "none",
			},
			Kafka: clowder.KafkaConfig{
				Mode: "none",
			},
			ObjectStore: clowder.ObjectStoreConfig{
				Mode: "none",
			},
			InMemoryDB: clowder.InMemoryDBConfig{
				Mode: "none",
			},
			Metrics: clowder.MetricsConfig{
				Mode: "none",
			},
		},
	}

	// Create LimitRange for testing
	testLimitRange := core.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name: "resource-limits",
		},
		Spec: core.LimitRangeSpec{
			Limits: []core.LimitRangeItem{
				{
					Type: core.LimitTypeContainer,
					Default: map[core.ResourceName]resource.Quantity{
						core.ResourceCPU:    resource.MustParse("200m"),
						core.ResourceMemory: resource.MustParse("512Mi"),
					},
					DefaultRequest: map[core.ResourceName]resource.Quantity{
						core.ResourceCPU:    resource.MustParse("100m"),
						core.ResourceMemory: resource.MustParse("384Mi"),
					},
				},
			},
		},
	}

	// Create ResourceQuotas for testing
	testResourceQuotas := core.ResourceQuotaList{
		Items: []core.ResourceQuota{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "compute-resources",
				},
				Spec: core.ResourceQuotaSpec{
					Hard: map[core.ResourceName]resource.Quantity{
						"limits.cpu":    resource.MustParse("32"),
						"limits.memory": resource.MustParse("64Gi"),
					},
				},
			},
		},
	}

	testPoolSpec := &crd.NamespacePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: crd.NamespacePoolSpec{
			Size:             2,
			Local:            true,
			ClowdEnvironment: testClowdEnvSpec,
			LimitRange:       testLimitRange,
			ResourceQuotas:   testResourceQuotas,
		},
	}

	err = k8sClient.Create(context.Background(), testPoolSpec)
	Expect(err).ToNot(HaveOccurred())

	testPoolSpec2 := &crd.NamespacePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "minimal",
		},
		Spec: crd.NamespacePoolSpec{
			Size:             2,
			Local:            true,
			ClowdEnvironment: testClowdEnvSpec,
			LimitRange:       testLimitRange,
			ResourceQuotas:   testResourceQuotas,
		},
	}

	err = k8sClient.Create(context.Background(), testPoolSpec2)
	Expect(err).ToNot(HaveOccurred())

	testPoolSpec3 := &crd.NamespacePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "limit",
		},
		Spec: crd.NamespacePoolSpec{
			Size:             2,
			SizeLimit:        utils.IntPtr(3),
			Local:            true,
			ClowdEnvironment: testClowdEnvSpec,
			LimitRange:       testLimitRange,
			ResourceQuotas:   testResourceQuotas,
		},
	}

	err = k8sClient.Create(context.Background(), testPoolSpec3)
	Expect(err).ToNot(HaveOccurred())

	poller := Poller{
		client:             k8sManager.GetClient(),
		activeReservations: make(map[string]metav1.Time),
		log:                ctrl.Log.WithName("Poller"),
	}

	err = (&NamespaceReservationReconciler{
		client: k8sManager.GetClient(),
		scheme: k8sManager.GetScheme(),
		poller: &poller,
		log:    ctrl.Log.WithName("controllers").WithName("ReservationController"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&NamespacePoolReconciler{
		client: k8sManager.GetClient(),
		scheme: k8sManager.GetScheme(),
		log:    ctrl.Log.WithName("controllers").WithName("NamespacePoolController"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ClowdenvironmentReconciler{
		client: k8sManager.GetClient(),
		scheme: k8sManager.GetScheme(),
		log:    ctrl.Log.WithName("controllers").WithName("ClowdEnvController"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		ctx, cancel := context.WithCancel(context.Background())
		stopController = cancel
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	go populateClowdEnvStatus(k8sManager.GetClient())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if stopController != nil {
		stopController()
	}
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// Helper function tests
var _ = Describe("Helper Functions Unit Tests", func() {
	Context("When testing annotation helpers", func() {
		It("Should create proper initial annotations", func() {
			annotations := helpers.CreateInitialAnnotations()

			Expect(annotations).To(HaveKey("env-status"))
			Expect(annotations).To(HaveKey("reserved"))
			Expect(annotations["env-status"]).To(Equal("creating"))
			Expect(annotations["reserved"]).To(Equal("false"))
		})

		It("Should create proper initial labels", func() {
			poolName := "test-pool"
			labels := helpers.CreateInitialLabels(poolName)

			Expect(labels).To(HaveKey("operator-ns"))
			Expect(labels).To(HaveKey("pool"))
			Expect(labels["operator-ns"]).To(Equal("true"))
			Expect(labels["pool"]).To(Equal(poolName))
		})

		It("Should convert custom annotations to map properly", func() {
			annotation := helpers.CustomAnnotation{
				Annotation: "test-key",
				Value:      "test-value",
			}

			annotationMap := annotation.ToMap()
			Expect(annotationMap).To(HaveLen(1))
			Expect(annotationMap["test-key"]).To(Equal("test-value"))
		})

		It("Should convert custom labels to map properly", func() {
			label := helpers.CustomLabel{
				Label: "test-label",
				Value: "test-value",
			}

			labelMap := label.ToMap()
			Expect(labelMap).To(HaveLen(1))
			Expect(labelMap["test-label"]).To(Equal("test-value"))
		})

		It("Should provide predefined annotation constants", func() {
			By("Testing env status annotations")
			Expect(helpers.AnnotationEnvReady.Annotation).To(Equal("env-status"))
			Expect(helpers.AnnotationEnvReady.Value).To(Equal("ready"))

			Expect(helpers.AnnotationEnvCreating.Annotation).To(Equal("env-status"))
			Expect(helpers.AnnotationEnvCreating.Value).To(Equal("creating"))

			Expect(helpers.AnnotationEnvError.Annotation).To(Equal("env-status"))
			Expect(helpers.AnnotationEnvError.Value).To(Equal("error"))

			Expect(helpers.AnnotationEnvDeleting.Annotation).To(Equal("env-status"))
			Expect(helpers.AnnotationEnvDeleting.Value).To(Equal("deleting"))

			By("Testing reserved annotations")
			Expect(helpers.AnnotationReservedTrue.Annotation).To(Equal("reserved"))
			Expect(helpers.AnnotationReservedTrue.Value).To(Equal("true"))

			Expect(helpers.AnnotationReservedFalse.Annotation).To(Equal("reserved"))
			Expect(helpers.AnnotationReservedFalse.Value).To(Equal("false"))

			By("Testing label constants")
			Expect(helpers.LabelOperatorNamespaceTrue.Label).To(Equal("operator-ns"))
			Expect(helpers.LabelOperatorNamespaceTrue.Value).To(Equal("true"))
		})
	})

	Context("When testing pool helper functions", func() {
		It("Should correctly determine if pool is at limit", func() {
			Expect(helpers.IsPoolAtLimit(0, 0)).To(BeTrue())
			Expect(helpers.IsPoolAtLimit(5, 5)).To(BeTrue())
			Expect(helpers.IsPoolAtLimit(3, 5)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(7, 5)).To(BeFalse())
		})

		It("Should handle edge cases in pool limit checking", func() {
			Expect(helpers.IsPoolAtLimit(-1, 5)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(5, -1)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(-1, -1)).To(BeTrue())
			Expect(helpers.IsPoolAtLimit(0, 1)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(1, 0)).To(BeFalse())
		})

		It("Should calculate namespace quantity delta correctly", func() {
			By("Testing basic scenarios without size limit")
			// No size limit, need 2 more ready namespaces
			delta := helpers.CalculateNamespaceQuantityDelta(nil, 5, 3, 0, 0)
			Expect(delta).To(Equal(2))

			// No size limit, already at capacity
			delta = helpers.CalculateNamespaceQuantityDelta(nil, 3, 3, 0, 0)
			Expect(delta).To(Equal(0))

			// No size limit, excess ready namespaces
			delta = helpers.CalculateNamespaceQuantityDelta(nil, 2, 5, 0, 0)
			Expect(delta).To(Equal(-3))

			By("Testing scenarios with size limit")
			// Size limit allows for creation
			delta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(10), 5, 2, 1, 2)
			Expect(delta).To(Equal(2))

			// Size limit blocks creation
			delta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 5, 1, 1, 2)
			Expect(delta).To(Equal(-1))

			// Size limit exactly met
			delta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(5), 3, 2, 0, 3)
			Expect(delta).To(Equal(1))
		})

		It("Should handle complex namespace quantity calculations", func() {
			By("Testing with creating namespaces counted")
			// Creating namespaces should be counted as part of queue
			delta := helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(5), 3, 1, 2, 1)
			Expect(delta).To(Equal(0)) // 1 ready + 2 creating = 3, so no more needed

			By("Testing size limit prioritization")
			// Size limit should override size when more restrictive
			delta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(2), 10, 0, 0, 0)
			Expect(delta).To(Equal(2)) // Limited by size limit, not size

			By("Testing negative deltas for cleanup")
			// Should return negative when over size limit
			delta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 3, 0, 2)
			Expect(delta).To(Equal(-2)) // Total is 5, limit is 3, so remove 2
		})
	})

	Context("When testing namespace utility functions", func() {
		It("Should handle ready status checking", func() {
			ctx := context.Background()

			// Create a test namespace
			ns := core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ready-status",
					Labels: map[string]string{
						"pool": "test-pool",
					},
					Annotations: map[string]string{
						"env-status": "ready",
					},
				},
			}

			err := k8sClient.Create(ctx, &ns)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			// Test CheckReadyStatus function
			var ready []core.Namespace
			result := helpers.CheckReadyStatus("test-pool", ns, ready)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("test-ready-status"))

			// Test with non-matching pool
			result = helpers.CheckReadyStatus("different-pool", ns, ready)
			Expect(result).To(HaveLen(0))

			// Test with non-ready status
			ns.Annotations["env-status"] = "creating"
			result = helpers.CheckReadyStatus("test-pool", ns, ready)
			Expect(result).To(HaveLen(0))
		})

		It("Should create proper reservation objects", func() {
			By("Testing NewReservation helper")
			res := helpers.NewReservation("test-res", "1h", "test-user", "test-pool")

			Expect(res.Name).To(Equal("test-res"))
			Expect(res.Spec.Requester).To(Equal("test-user"))
			Expect(res.Spec.Pool).To(Equal("test-pool"))
			Expect(*res.Spec.Duration).To(Equal("1h"))
			Expect(res.TypeMeta.Kind).To(Equal("NamespaceReservation"))
		})

		It("Should handle reservation creation with empty values", func() {
			By("Testing NewReservation with empty duration")
			res := helpers.NewReservation("test-empty", "", "user", "pool")
			Expect(res.Spec.Duration).To(BeNil())

			By("Testing NewReservation with empty pool")
			res = helpers.NewReservation("test-empty-pool", "1h", "user", "")
			Expect(res.Spec.Pool).To(Equal(""))

			By("Testing NewReservation with empty requester")
			res = helpers.NewReservation("test-empty-user", "1h", "", "pool")
			Expect(res.Spec.Requester).To(Equal(""))
		})
	})

	Context("When testing constant definitions", func() {
		It("Should have correct constant values", func() {
			By("Testing annotation constants")
			Expect(helpers.AnnotationEnvStatus).To(Equal("env-status"))
			Expect(helpers.AnnotationReserved).To(Equal("reserved"))
			Expect(helpers.CompletionTime).To(Equal("completion-time"))

			By("Testing status value constants")
			Expect(helpers.EnvStatusCreating).To(Equal("creating"))
			Expect(helpers.EnvStatusDeleting).To(Equal("deleting"))
			Expect(helpers.EnvStatusError).To(Equal("error"))
			Expect(helpers.EnvStatusReady).To(Equal("ready"))

			By("Testing label constants")
			Expect(helpers.LabelOperatorNS).To(Equal("operator-ns"))
			Expect(helpers.LabelPool).To(Equal("pool"))

			By("Testing kind constants")
			Expect(helpers.KindNamespacePool).To(Equal("NamespacePool"))

			By("Testing namespace constants")
			Expect(helpers.NamespaceEphemeralBase).To(Equal("ephemeral-base"))

			By("Testing secret constants")
			Expect(helpers.BonfireGinoreSecret).To(Equal("bonfire.ignore"))
			Expect(helpers.OpenShiftVaultSecretsSecret).To(Equal("openshift-vault-secrets"))
			Expect(helpers.QontractIntegrationSecret).To(Equal("qontract.integration"))

			By("Testing boolean constants")
			Expect(helpers.TrueValue).To(Equal("true"))
			Expect(helpers.FalseValue).To(Equal("false"))
		})
	})
})

// Poller tests
var _ = Describe("Poller Unit Tests", func() {
	Context("When testing Poller functionality", func() {
		It("Should correctly identify expired namespaces", func() {
			poller := &Poller{}

			By("Testing with future expiration time")
			futureTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
			Expect(poller.namespaceIsExpired(futureTime)).To(BeFalse())

			By("Testing with past expiration time")
			pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			Expect(poller.namespaceIsExpired(pastTime)).To(BeTrue())

			By("Testing with zero time")
			zeroTime := metav1.Time{}
			Expect(poller.namespaceIsExpired(zeroTime)).To(BeFalse())

			By("Testing with current time (should be expired)")
			currentTime := metav1.NewTime(time.Now().Add(-1 * time.Second))
			Expect(poller.namespaceIsExpired(currentTime)).To(BeTrue())
		})

		It("Should handle edge cases in expiration checking", func() {
			poller := &Poller{}

			By("Testing with very recent past time")
			recentPast := metav1.NewTime(time.Now().Add(-1 * time.Millisecond))
			Expect(poller.namespaceIsExpired(recentPast)).To(BeTrue())

			By("Testing with very near future time")
			nearFuture := metav1.NewTime(time.Now().Add(1 * time.Millisecond))
			Expect(poller.namespaceIsExpired(nearFuture)).To(BeFalse())
		})

		It("Should populate active reservations correctly", func() {
			ctx := context.Background()
			poller := &Poller{
				client:             k8sClient,
				activeReservations: make(map[string]metav1.Time),
				log:                ctrl.Log.WithName("TestPoller"),
			}

			By("Creating test reservations")
			activeRes := helpers.NewReservation("active-res", "1h", "test-user", "default")
			err := k8sClient.Create(ctx, activeRes)
			Expect(err).NotTo(HaveOccurred())

			// Update to active state
			Eventually(func() error {
				updatedRes := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "active-res"}, updatedRes)
				if err != nil {
					return err
				}
				updatedRes.Status.State = "active"
				updatedRes.Status.Expiration = metav1.NewTime(time.Now().Add(1 * time.Hour))
				return k8sClient.Status().Update(ctx, updatedRes)
			}, time.Second*10, time.Millisecond*100).Should(Succeed())

			By("Testing populateActiveReservations")
			err = poller.populateActiveReservations(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying active reservation was added")
			Eventually(func() bool {
				_, exists := poller.activeReservations["active-res"]
				return exists
			}, time.Second*5, time.Millisecond*100).Should(BeTrue())
		})

		It("Should get existing reservations", func() {
			ctx := context.Background()
			poller := &Poller{
				client:             k8sClient,
				activeReservations: make(map[string]metav1.Time),
				log:                ctrl.Log.WithName("TestPoller"),
			}

			resList, err := poller.getExistingReservations(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resList).NotTo(BeNil())
			Expect(len(resList.Items)).To(BeNumerically(">=", 0))
		})
	})

	Context("When testing Poller edge cases", func() {
		It("Should handle empty reservation lists", func() {
			ctx := context.Background()
			poller := &Poller{
				client:             k8sClient,
				activeReservations: make(map[string]metav1.Time),
				log:                ctrl.Log.WithName("TestPoller"),
			}

			resList, err := poller.getExistingReservations(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resList.Items).NotTo(BeNil())
		})

		It("Should handle activeReservations map operations", func() {
			poller := &Poller{
				activeReservations: make(map[string]metav1.Time),
				log:                ctrl.Log.WithName("TestPoller"),
			}

			By("Testing map operations")
			testTime := metav1.NewTime(time.Now())
			poller.activeReservations["test-res"] = testTime

			Expect(poller.activeReservations).To(HaveKey("test-res"))
			Expect(poller.activeReservations["test-res"]).To(Equal(testTime))

			delete(poller.activeReservations, "test-res")
			Expect(poller.activeReservations).NotTo(HaveKey("test-res"))
		})

		It("Should verify poll cycle constant", func() {
			Expect(PollCycle).To(Equal(time.Duration(10)))
		})
	})
})

// Integration tests for complex scenarios
var _ = Describe("Integration Test Scenarios", func() {
	Context("When testing complex workflows", func() {
		It("Should handle multiple pools with different configurations", func() {
			ctx := context.Background()

			By("Verifying different pool configurations exist")
			pools := []string{"default", "minimal", "limit"}
			for _, poolName := range pools {
				pool := &crd.NamespacePool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: poolName}, pool)
				Expect(err).NotTo(HaveOccurred())
				Expect(pool.Name).To(Equal(poolName))
			}

			By("Verifying pool-specific configurations")
			limitPool := &crd.NamespacePool{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, limitPool)
			Expect(err).NotTo(HaveOccurred())
			Expect(limitPool.Spec.SizeLimit).NotTo(BeNil())
			Expect(*limitPool.Spec.SizeLimit).To(Equal(3))
		})

		It("Should handle cross-pool reservation scenarios", func() {
			ctx := context.Background()

			By("Creating reservations across different pools")
			res1 := helpers.NewReservation("cross-pool-1", "30m", "user1", "default")
			res2 := helpers.NewReservation("cross-pool-2", "30m", "user2", "minimal")
			res3 := helpers.NewReservation("cross-pool-3", "30m", "user3", "limit")

			Expect(k8sClient.Create(ctx, res1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res2)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res3)).Should(Succeed())

			By("Verifying reservations are processed by correct pools")
			Eventually(func() bool {
				var allProcessed bool = true

				for _, resName := range []string{"cross-pool-1", "cross-pool-2", "cross-pool-3"} {
					res := &crd.NamespaceReservation{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
					if err != nil || (res.Status.State != "active" && res.Status.State != "waiting") {
						allProcessed = false
						break
					}
				}

				return allProcessed
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())
		})

		It("Should handle stress testing with multiple concurrent operations", func() {
			ctx := context.Background()

			By("Creating multiple reservations simultaneously")
			resNames := []string{}
			for i := 0; i < 5; i++ {
				resName := fmt.Sprintf("stress-test-%d", i)
				resNames = append(resNames, resName)

				res := helpers.NewReservation(resName, "15m", fmt.Sprintf("stress-user-%d", i), "default")
				Expect(k8sClient.Create(ctx, res)).Should(Succeed())
			}

			By("Verifying all reservations are eventually processed")
			Eventually(func() int {
				processedCount := 0
				for _, resName := range resNames {
					res := &crd.NamespaceReservation{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
					if err == nil && (res.Status.State == "active" || res.Status.State == "waiting") {
						processedCount++
					}
				}
				return processedCount
			}, time.Second*30, time.Millisecond*250).Should(Equal(5))

			By("Cleaning up stress test reservations")
			for _, resName := range resNames {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				if err == nil {
					k8sClient.Delete(ctx, res)
				}
			}
		})
	})
})
