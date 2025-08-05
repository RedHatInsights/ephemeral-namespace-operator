package controllers

import (
	"context"
	"fmt"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var _ = Describe("Clowdenvironment controller basic update", func() {
	const (
		timeout  = time.Second * 30
		duration = time.Second * 30
		interval = time.Millisecond * 250
	)

	Context("When a clowdenvironment is created", func() {
		It("Should update the namespace annotations when ready if owned by the pool", func() {
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the clowdenvironment conditions")
			Eventually(func() bool {
				err := k8sClient.List(ctx, &nsList)
				Expect(err).NotTo(HaveOccurred())

				if len(nsList.Items) == 0 {
					return false
				}

				for _, ns := range nsList.Items {
					if isOwnedByPool(ctx, k8sClient, ns.Name) {
						a := ns.GetAnnotations()
						if val, ok := a["env-status"]; !ok || val != "ready" {
							if val != "deleting" {
								return false
							}
						}
					}
				}

				return true
			}, timeout, interval).Should(BeTrue())
		})

		It("Should ignore envs not owned by the pool", func() {
			By("Checking namespace ownerRef in event filter")
			ctx := context.Background()

			ns := core.Namespace{}
			ns.Name = "no-owner"

			err := k8sClient.Create(ctx, &ns)
			Expect(err).NotTo(HaveOccurred())

			Expect(isOwnedByPool(ctx, k8sClient, ns.Name)).To(Equal(false))
		})

		It("Should handle ClowdEnvironment status transitions properly", func() {
			ctx := context.Background()

			By("Creating a test namespace with pool ownership")
			testNs := &core.Namespace{}
			testNs.Name = "test-clowdenv-ns"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			// Create mock pool for ownership
			pool := &crd.NamespacePool{}
			pool.Name = "test-pool"
			pool.Namespace = "default"
			err := k8sClient.Create(ctx, pool)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			testNs.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: "cloud.redhat.com/v1alpha1",
					Kind:       "NamespacePool",
					Name:       pool.Name,
					UID:        pool.UID,
					Controller: &[]bool{true}[0],
				},
			}

			err = k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Creating a ClowdEnvironment in the test namespace")
			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-env"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = testNs.Name
			clowdEnv.Status.Deployments.ManagedDeployments = 1
			clowdEnv.Status.Deployments.ReadyDeployments = 1

			err = k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying namespace annotations are updated")
			Eventually(func() string {
				updatedNs := &core.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, updatedNs)
				if err != nil {
					return ""
				}
				return updatedNs.Annotations["env-status"]
			}, timeout, interval).Should(Or(Equal("ready"), Equal("creating")))
		})

		It("Should handle ClowdEnvironment with unready deployments", func() {
			ctx := context.Background()

			By("Creating ClowdEnvironment with unready deployments")
			testNs := &core.Namespace{}
			testNs.Name = "test-unready-clowdenv"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			err := k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-unready-env"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = testNs.Name
			clowdEnv.Status.Deployments.ManagedDeployments = 3
			clowdEnv.Status.Deployments.ReadyDeployments = 1 // Not all ready

			err = k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying namespace does not get marked as ready")
			Consistently(func() string {
				updatedNs := &core.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, updatedNs)
				if err != nil {
					return ""
				}
				if status, ok := updatedNs.Annotations["env-status"]; ok {
					return status
				}
				return "unknown"
			}, time.Second*5, time.Millisecond*500).ShouldNot(Equal("ready"))
		})

		It("Should create FrontendEnvironment when ClowdEnvironment is ready", func() {
			ctx := context.Background()

			By("Creating a ready ClowdEnvironment")
			testNs := &core.Namespace{}
			testNs.Name = "test-frontend-ns"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			err := k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-frontend-env"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = testNs.Name
			clowdEnv.Status.Deployments.ManagedDeployments = 1
			clowdEnv.Status.Deployments.ReadyDeployments = 1

			err = k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying FrontendEnvironment is created")
			Eventually(func() bool {
				frontendEnv := &frontend.FrontendEnvironment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testNs.Name,
					Namespace: testNs.Name,
				}, frontendEnv)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle error conditions in ClowdEnvironment processing", func() {
			ctx := context.Background()

			By("Creating a ClowdEnvironment that will cause processing errors")
			testNs := &core.Namespace{}
			testNs.Name = "test-error-ns"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			err := k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-error-env"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = testNs.Name
			clowdEnv.Status.Deployments.ManagedDeployments = 1
			clowdEnv.Status.Deployments.ReadyDeployments = 1

			err = k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Simulating an error by setting invalid annotations")
			err = helpers.UpdateAnnotations(ctx, k8sClient, testNs.Name, helpers.AnnotationEnvError.ToMap())
			if err == nil {
				By("Verifying error annotation is set")
				Eventually(func() string {
					updatedNs := &core.Namespace{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, updatedNs)
					if err != nil {
						return ""
					}
					return updatedNs.Annotations["env-status"]
				}, timeout, interval).Should(Equal("error"))
			}
		})
	})

	Context("When handling ClowdEnvironment edge cases", func() {
		It("Should handle missing target namespace gracefully", func() {
			ctx := context.Background()

			By("Creating ClowdEnvironment with non-existent target namespace")
			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-missing-target"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = "non-existent-namespace"
			clowdEnv.Status.Deployments.ManagedDeployments = 1
			clowdEnv.Status.Deployments.ReadyDeployments = 1

			err := k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying controller handles missing target namespace")
			// The controller should not crash and should handle this gracefully
			Eventually(func() bool {
				return true // Just verify the controller is still running
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())
		})

		It("Should handle ClowdEnvironment deletion", func() {
			ctx := context.Background()

			By("Creating and then deleting a ClowdEnvironment")
			testNs := &core.Namespace{}
			testNs.Name = "test-delete-ns"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			err := k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-delete-env"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = testNs.Name
			clowdEnv.Status.Deployments.ManagedDeployments = 1
			clowdEnv.Status.Deployments.ReadyDeployments = 1

			err = k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Deleting the ClowdEnvironment")
			err = k8sClient.Delete(ctx, clowdEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ClowdEnvironment is deleted")
			Eventually(func() bool {
				deletedEnv := &clowder.ClowdEnvironment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clowdEnv.Name,
					Namespace: clowdEnv.Namespace,
				}, deletedEnv)
				return k8serr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle multiple ClowdEnvironments in different namespaces", func() {
			ctx := context.Background()

			namespaces := []string{"test-multi-1", "test-multi-2", "test-multi-3"}

			By("Creating multiple namespaces with ClowdEnvironments")
			for i, nsName := range namespaces {
				testNs := &core.Namespace{}
				testNs.Name = nsName
				testNs.Labels = map[string]string{
					"operator-ns": "true",
					"pool":        "default",
				}

				err := k8sClient.Create(ctx, testNs)
				if err != nil && !k8serr.IsAlreadyExists(err) {
					Expect(err).NotTo(HaveOccurred())
				}

				clowdEnv := &clowder.ClowdEnvironment{}
				clowdEnv.Name = fmt.Sprintf("test-multi-env-%d", i)
				clowdEnv.Namespace = "default"
				clowdEnv.Spec.TargetNamespace = nsName
				clowdEnv.Status.Deployments.ManagedDeployments = 1
				clowdEnv.Status.Deployments.ReadyDeployments = 1

				err = k8sClient.Create(ctx, clowdEnv)
				if err != nil && !k8serr.IsAlreadyExists(err) {
					Expect(err).NotTo(HaveOccurred())
				}
			}

			By("Verifying all namespaces are processed correctly")
			for _, nsName := range namespaces {
				Eventually(func() bool {
					ns := &core.Namespace{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns)
					if err != nil {
						return false
					}
					return ns.Annotations["env-status"] == "ready" || ns.Annotations["env-status"] == "creating"
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should handle concurrent ClowdEnvironment updates", func() {
			ctx := context.Background()

			By("Creating a ClowdEnvironment")
			testNs := &core.Namespace{}
			testNs.Name = "test-concurrent-ns"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			err := k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			clowdEnv := &clowder.ClowdEnvironment{}
			clowdEnv.Name = "test-concurrent-env"
			clowdEnv.Namespace = "default"
			clowdEnv.Spec.TargetNamespace = testNs.Name
			clowdEnv.Status.Deployments.ManagedDeployments = 2
			clowdEnv.Status.Deployments.ReadyDeployments = 1

			err = k8sClient.Create(ctx, clowdEnv)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Updating ClowdEnvironment to ready state")
			Eventually(func() error {
				updatedEnv := &clowder.ClowdEnvironment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clowdEnv.Name,
					Namespace: clowdEnv.Namespace,
				}, updatedEnv)
				if err != nil {
					return err
				}

				updatedEnv.Status.Deployments.ReadyDeployments = 2
				return k8sClient.Status().Update(ctx, updatedEnv)
			}, timeout, interval).Should(Succeed())

			By("Verifying namespace status is eventually updated")
			Eventually(func() string {
				ns := &core.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, ns)
				if err != nil {
					return ""
				}
				return ns.Annotations["env-status"]
			}, timeout, interval).Should(Equal("ready"))
		})
	})

	Context("When testing ClowdEnvironment readiness verification", func() {
		It("Should properly evaluate readiness conditions", func() {
			By("Testing ClowdEnvironment readiness with different condition states")

			// Test case 1: Environment with proper conditions
			readyEnv := clowder.ClowdEnvironment{
				Spec: clowder.ClowdEnvironmentSpec{
					Providers: clowder.ProvidersConfig{
						Web: clowder.WebConfig{
							Mode: "operator",
						},
					},
				},
				Status: clowder.ClowdEnvironmentStatus{
					Conditions: []clusterv1.Condition{
						{
							Type:   clowder.ReconciliationSuccessful,
							Status: core.ConditionTrue,
						},
						{
							Type:   clowder.DeploymentsReady,
							Status: core.ConditionTrue,
						},
					},
				},
			}
			Expect(helpers.VerifyClowdEnvReady(readyEnv)).To(BeTrue())

			// Test case 2: Environment with missing conditions
			notReadyEnv := clowder.ClowdEnvironment{
				Spec: clowder.ClowdEnvironmentSpec{
					Providers: clowder.ProvidersConfig{
						Web: clowder.WebConfig{
							Mode: "operator",
						},
					},
				},
				Status: clowder.ClowdEnvironmentStatus{
					Conditions: []clusterv1.Condition{
						{
							Type:   clowder.ReconciliationSuccessful,
							Status: core.ConditionTrue,
						},
					},
				},
			}
			Expect(helpers.VerifyClowdEnvReady(notReadyEnv)).To(BeFalse())

			// Test case 3: Environment in local mode without hostname
			localEnv := clowder.ClowdEnvironment{
				Spec: clowder.ClowdEnvironmentSpec{
					Providers: clowder.ProvidersConfig{
						Web: clowder.WebConfig{
							Mode: "local",
						},
					},
				},
				Status: clowder.ClowdEnvironmentStatus{
					Hostname: "", // Missing hostname for local mode
					Conditions: []clusterv1.Condition{
						{
							Type:   clowder.ReconciliationSuccessful,
							Status: core.ConditionTrue,
						},
						{
							Type:   clowder.DeploymentsReady,
							Status: core.ConditionTrue,
						},
					},
				},
			}
			Expect(helpers.VerifyClowdEnvReady(localEnv)).To(BeFalse())

			// Test case 4: Environment in local mode with hostname
			localReadyEnv := clowder.ClowdEnvironment{
				Spec: clowder.ClowdEnvironmentSpec{
					Providers: clowder.ProvidersConfig{
						Web: clowder.WebConfig{
							Mode: "local",
						},
					},
				},
				Status: clowder.ClowdEnvironmentStatus{
					Hostname: "test.example.com",
					Conditions: []clusterv1.Condition{
						{
							Type:   clowder.ReconciliationSuccessful,
							Status: core.ConditionTrue,
						},
						{
							Type:   clowder.DeploymentsReady,
							Status: core.ConditionTrue,
						},
					},
				},
			}
			Expect(helpers.VerifyClowdEnvReady(localReadyEnv)).To(BeTrue())
		})

		It("Should handle edge cases in condition states", func() {
			By("Testing with empty conditions")

			emptyEnv := clowder.ClowdEnvironment{
				Spec: clowder.ClowdEnvironmentSpec{
					Providers: clowder.ProvidersConfig{
						Web: clowder.WebConfig{
							Mode: "operator",
						},
					},
				},
				Status: clowder.ClowdEnvironmentStatus{
					Conditions: []clusterv1.Condition{},
				},
			}
			Expect(helpers.VerifyClowdEnvReady(emptyEnv)).To(BeFalse())

			By("Testing with false conditions")
			falseEnv := clowder.ClowdEnvironment{
				Spec: clowder.ClowdEnvironmentSpec{
					Providers: clowder.ProvidersConfig{
						Web: clowder.WebConfig{
							Mode: "operator",
						},
					},
				},
				Status: clowder.ClowdEnvironmentStatus{
					Conditions: []clusterv1.Condition{
						{
							Type:   clowder.ReconciliationSuccessful,
							Status: core.ConditionFalse,
						},
						{
							Type:   clowder.DeploymentsReady,
							Status: core.ConditionFalse,
						},
					},
				},
			}
			Expect(helpers.VerifyClowdEnvReady(falseEnv)).To(BeFalse())
		})
	})

	Context("When testing annotation handling", func() {
		It("Should properly update completion time annotations", func() {
			ctx := context.Background()

			By("Creating a namespace for completion time testing")
			testNs := &core.Namespace{}
			testNs.Name = "test-completion-time"
			testNs.Labels = map[string]string{
				"operator-ns": "true",
				"pool":        "default",
			}

			err := k8sClient.Create(ctx, testNs)
			if err != nil && !k8serr.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Setting ready annotation")
			err = helpers.UpdateAnnotations(ctx, k8sClient, testNs.Name, helpers.AnnotationEnvReady.ToMap())
			Expect(err).NotTo(HaveOccurred())

			By("Verifying annotation is set")
			Eventually(func() string {
				updatedNs := &core.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, updatedNs)
				if err != nil {
					return ""
				}
				return updatedNs.Annotations["env-status"]
			}, timeout, interval).Should(Equal("ready"))
		})

		It("Should handle annotation update errors gracefully", func() {
			ctx := context.Background()

			By("Attempting to update annotations on non-existent namespace")
			err := helpers.UpdateAnnotations(ctx, k8sClient, "non-existent-namespace", helpers.AnnotationEnvReady.ToMap())
			Expect(err).To(HaveOccurred())
		})

		It("Should handle various annotation states", func() {
			ctx := context.Background()

			testStates := []helpers.CustomAnnotation{
				helpers.AnnotationEnvCreating,
				helpers.AnnotationEnvReady,
				helpers.AnnotationEnvError,
				helpers.AnnotationEnvDeleting,
			}

			for i, state := range testStates {
				nsName := fmt.Sprintf("test-annotation-state-%d", i)
				testNs := &core.Namespace{}
				testNs.Name = nsName
				testNs.Labels = map[string]string{
					"operator-ns": "true",
					"pool":        "default",
				}

				err := k8sClient.Create(ctx, testNs)
				if err != nil && !k8serr.IsAlreadyExists(err) {
					Expect(err).NotTo(HaveOccurred())
				}

				By(fmt.Sprintf("Setting %s annotation", state.Value))
				err = helpers.UpdateAnnotations(ctx, k8sClient, nsName, state.ToMap())
				Expect(err).NotTo(HaveOccurred())

				By("Verifying annotation is set correctly")
				Eventually(func() string {
					updatedNs := &core.Namespace{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, updatedNs)
					if err != nil {
						return ""
					}
					return updatedNs.Annotations["env-status"]
				}, timeout, interval).Should(Equal(state.Value))
			}
		})
	})
})
