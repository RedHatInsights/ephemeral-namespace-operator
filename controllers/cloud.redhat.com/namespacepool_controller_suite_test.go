package controllers

import (
	"context"
	"fmt"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Pool controller basic functionality", func() {
	const (
		timeout  = time.Second * 35
		duration = time.Second * 30
		interval = time.Millisecond * 30
	)

	Context("When a pool is reconciled", func() {
		It("Should reconcile the number of managed namespaces for each pool", func() {
			By("Checking the total number of owned namespaces")
			ctx := context.Background()
			pool := crd.NamespacePool{}

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.List(ctx, &nsList)
				ownedCount := 0
				for _, ns := range nsList.Items {
					if isOwnedBySpecificPool(ctx, k8sClient, ns.Name, pool.UID) {
						ownedCount++
					}
				}
				Expect(err).NotTo(HaveOccurred())

				return ownedCount == pool.Spec.Size
			}, timeout, interval).Should(BeTrue())
		})
	})
})

var _ = Describe("Ensure new namespaces are setup properly", func() {
	Context("When a new namespace is created", func() {
		It("Should contain necessary labels and annotations", func() {
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" {
						Expect(ns.Labels["operator-ns"]).To(Equal("true"))

						_, ok := ns.Labels["pool"]
						Expect(ok).To(BeTrue())

						_, ok = ns.Annotations["env-status"]
						Expect(ok).To(BeTrue())

						Expect(ns.Annotations["reserved"]).To(Equal("false"))
					}
				}
			}
		})

		It("Should have proper owner references set", func() {
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" {
						Expect(owner.APIVersion).To(Equal("cloud.redhat.com/v1alpha1"))
						Expect(owner.Controller).NotTo(BeNil())
						Expect(*owner.Controller).To(BeTrue())
						Expect(owner.BlockOwnerDeletion).NotTo(BeNil())
						Expect(*owner.BlockOwnerDeletion).To(BeTrue())
					}
				}
			}
		})

		It("Should handle initial namespace creation status properly", func() {
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" {
						status := ns.Annotations["env-status"]
						Expect(status).To(Or(Equal("creating"), Equal("ready"), Equal("error")))
					}
				}
			}
		})
	})
})

var _ = Describe("Namespace creation should not exceed the pool size limit if one is defined", func() {
	Context("If a pool has the optional size limit attribute, it should not exceed that limit", func() {
		It("should create the number of namespaces equal to the size limit defined", func() {
			By("Comparing pool status to pool size")
			ctx := context.Background()
			pool := crd.NamespacePool{}

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Spec.Size == (pool.Status.Ready + pool.Status.Creating)
			}, timeout, interval).Should(BeTrue())

			By("Ensuring that if the limit is decreased, excess namespaces are deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			pool.Spec.Size--

			Eventually(func() error {
				return k8sClient.Update(ctx, &pool)
			}, timeout, interval).Should(BeNil())

			Eventually(func() int {
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Status.Ready + pool.Status.Creating
			}, timeout, interval).Should(Equal(1))

			By("Ensuring that if the limit is increased, more namespaces are created")
			pool.Spec.Size++

			Eventually(func() error {
				return k8sClient.Update(ctx, &pool)
			}, timeout, interval).Should(BeNil())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Spec.Size == pool.Status.Ready
			}, timeout, interval).Should(BeTrue())

			pool.Spec.Size--

			Eventually(func() error {
				return k8sClient.Update(ctx, &pool)
			}, timeout, interval).Should(BeNil())
		})

		It("Should handle pool size changes gracefully", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			originalSize := pool.Spec.Size

			By("Increasing pool size significantly")
			pool.Spec.Size = originalSize + 5
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())
				return pool.Status.Ready + pool.Status.Creating
			}, timeout, interval).Should(Equal(originalSize + 5))

			By("Decreasing pool size back to original")
			pool.Spec.Size = originalSize
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())
				return pool.Status.Ready + pool.Status.Creating
			}, timeout, interval).Should(Equal(originalSize))
		})

		It("Should handle zero pool size", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			originalSize := pool.Spec.Size

			By("Setting pool size to zero")
			pool.Spec.Size = 0
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())
				return pool.Status.Ready + pool.Status.Creating
			}, timeout, interval).Should(Equal(0))

			By("Restoring pool size")
			pool.Spec.Size = originalSize
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Rogue namespaces should be deleted", func() {
	Context("When a namespaces fails to be created properly", func() {
		It("Should delete the namespace with the `env-status: error` annotation", func() {
			By("Checking namespace annotations")
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			ownedNs := core.Namespace{}
			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" {
						ownedNs = ns
						break
					}
				}
			}

			err = helpers.UpdateAnnotations(ctx, k8sClient, ownedNs.Name, helpers.AnnotationEnvError.ToMap())
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: ownedNs.Name}, &ownedNs)
				if k8serr.IsNotFound(err) || ownedNs.Status.Phase == "Terminating" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle multiple error namespaces simultaneously", func() {
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			var errorNamespaces []core.Namespace
			count := 0
			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" && count < 2 {
						errorNamespaces = append(errorNamespaces, ns)
						count++
					}
				}
			}

			By("Marking multiple namespaces as error")
			for _, ns := range errorNamespaces {
				err = helpers.UpdateAnnotations(ctx, k8sClient, ns.Name, helpers.AnnotationEnvError.ToMap())
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying all error namespaces are deleted")
			for _, ns := range errorNamespaces {
				Eventually(func() bool {
					var checkNs core.Namespace
					err := k8sClient.Get(ctx, types.NamespacedName{Name: ns.Name}, &checkNs)
					return k8serr.IsNotFound(err) || checkNs.Status.Phase == "Terminating"
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should not delete namespaces with other status values", func() {
			ctx := context.Background()

			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			var testNamespace core.Namespace
			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" {
						testNamespace = ns
						break
					}
				}
			}

			By("Setting namespace status to 'creating'")
			err = helpers.UpdateAnnotations(ctx, k8sClient, testNamespace.Name, helpers.AnnotationEnvCreating.ToMap())
			Expect(err).NotTo(HaveOccurred())

			By("Verifying namespace is not deleted")
			Consistently(func() bool {
				var checkNs core.Namespace
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNamespace.Name}, &checkNs)
				return err == nil && checkNs.Status.Phase != "Terminating"
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())
		})
	})
})

var _ = Describe("When 'SizeLimit' is specified in the pool resource, a limit for namespace creation should occur", func() {
	Context("Ensure that a namespace creation limit occurs when a pool has the 'SizeLimit' attribute", func() {
		It("Should stop creating namespaces once the number of namespaces created equals the 'SizeLimit'", func() {
			By("Limiting created namespaces when the value of 'SizeLimit' has been reached")
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
			Expect(err).ToNot(HaveOccurred())

			resName6 := "limit-res-1"
			resName7 := "limit-res-2"

			res6 := helpers.NewReservation(resName6, "30m", "test-user-12", "", "limit")
			res7 := helpers.NewReservation(resName7, "30m", "test-user-13", "", "limit")

			Expect(k8sClient.Create(ctx, res6)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res7)).Should(Succeed())

			expected := fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", 1, 0, 2)

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				ready := pool.Status.Ready
				creating := pool.Status.Creating
				reserved := pool.Status.Reserved

				return fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", ready, creating, reserved)
			}, timeout, interval).Should(Equal(expected))

			By("Increasing the 'SizeLimit' for a pool resource when needed")
			pool.Spec.SizeLimit = utils.IntPtr(4)

			Eventually(func() error {
				return k8sClient.Update(ctx, &pool)
			}, timeout, interval).Should(BeNil())

			Expect(pool.Spec.SizeLimit).To(Equal(utils.IntPtr(4)))

			By("Creating more namespaces when necessary if the value of 'SizeLimit' had been increased")
			expected = fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", 2, 0, 2)

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				ready := pool.Status.Ready
				creating := pool.Status.Creating
				reserved := pool.Status.Reserved

				return fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", ready, creating, reserved)
			}, timeout, interval).Should(Equal(expected))

			By("Decreasing the 'SizeLimit' for a pool resource when needed")
			pool.Spec.SizeLimit = utils.IntPtr(3)

			Eventually(func() error {
				return k8sClient.Update(ctx, &pool)
			}, timeout, interval).Should(BeNil())

			Expect(pool.Spec.SizeLimit).To(Equal(utils.IntPtr(3)))

			By("Deleting namespaces when necessary if the value of 'SizeLimit' has decreased")
			expected = fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", 1, 0, 2)

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				ready := pool.Status.Ready
				creating := pool.Status.Creating
				reserved := pool.Status.Reserved

				return fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", ready, creating, reserved)
			}, timeout, interval).Should(Equal(expected))
		})

		It("Should handle SizeLimit changes with active reservations", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
			Expect(err).ToNot(HaveOccurred())

			originalSizeLimit := pool.Spec.SizeLimit

			By("Creating reservations that exceed current size limit")
			resNames := []string{"limit-test-1", "limit-test-2", "limit-test-3", "limit-test-4", "limit-test-5"}
			for i, resName := range resNames {
				res := helpers.NewReservation(resName, "30m", fmt.Sprintf("test-user-%d", i), "limit")
				Expect(k8sClient.Create(ctx, res)).Should(Succeed())
			}

			By("Verifying that only size limit number of reservations become active")
			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
				Expect(err).NotTo(HaveOccurred())
				return pool.Status.Reserved
			}, timeout, interval).Should(BeNumerically("<=", *originalSizeLimit))

			By("Increasing size limit to accommodate more reservations")
			pool.Spec.SizeLimit = utils.IntPtr(*originalSizeLimit + 3)
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
				Expect(err).NotTo(HaveOccurred())
				return pool.Status.Reserved
			}, timeout, interval).Should(BeNumerically("<=", *pool.Spec.SizeLimit))
		})

		It("Should handle SizeLimit set to zero", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
			Expect(err).ToNot(HaveOccurred())

			originalSizeLimit := pool.Spec.SizeLimit

			By("Setting SizeLimit to zero")
			pool.Spec.SizeLimit = utils.IntPtr(0)
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "limit"}, &pool)
				Expect(err).NotTo(HaveOccurred())
				return pool.Status.Ready + pool.Status.Creating
			}, timeout, interval).Should(Equal(0))

			By("Restoring original SizeLimit")
			pool.Spec.SizeLimit = originalSizeLimit
			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Edge cases for creating namespaces with pool limit set", func() {
	Context("Ensure that calculating namespace creation or deletion is handled properly", func() {
		It("Should calculate namespace creation, deletion, or return 0 if no changes are needed", func() {
			// With a size limit of 8, ready at 0, and total namespaces at 2; create 2 ready namespaces
			By("Testing edge case => SizeLimit: 8, Size: 2, Ready: 0, Creating: 0, Reserved: 2")
			namespaceDelta := helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(8), 2, 0, 0, 2)
			Expect(namespaceDelta).To(Equal(2))

			// With a size limit of 3, ready at 0, and total namespaces at 2; create 1 more ready namespace
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 2")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 2)
			Expect(namespaceDelta).To(Equal(1))

			// Total namespaces equals pool limit, therefore no more namespaces are created
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 3")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 3)
			Expect(namespaceDelta).To(Equal(0))

			// No size limit on pool but we are at the max for ready namespace size then we don't create more namespaces
			By("Testing edge case => SizeLimit: nil, Size: 2, Ready: 2, Creating: 0, Reserved: 2")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(nil, 2, 2, 0, 2)
			Expect(namespaceDelta).To(Equal(0))

			// With no ready, but two creating, size is already hit and no more are created
			By("Testing edge case => SizeLimit: 5, Size: 2, Ready: 0, Creating: 2, Reserved: 2")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(5), 2, 0, 2, 2)
			Expect(namespaceDelta).To(Equal(0))

			// If there are more namespaces then the size limit we reduce the ready namespace count from 2 to 1
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 2, Creating: 0, Reserved: 2")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 2, 0, 2)
			Expect(namespaceDelta).To(Equal(-1))

			// If 4 namespaces are reserved with pool size limit of 3, don't create more and don't remove any that are reserved
			// Since there are no ready namespaces to delete, this will not remove any namespaces
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 4")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 4)
			Expect(namespaceDelta).To(Equal(-1))

			// Create no more namespaces since 3 are active across ready, creating, and reserved
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 1, Creating: 1, Reserved: 1")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 1, 1, 1)
			Expect(namespaceDelta).To(Equal(0))

			// Ensure at startup that t
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 0")
			namespaceDelta = helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 0)
			Expect(namespaceDelta).To(Equal(2))
		})

		It("Should handle negative size values gracefully", func() {
			By("Testing with negative size")
			namespaceDelta := helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(5), -1, 0, 0, 0)
			Expect(namespaceDelta).To(Equal(-1))
		})

		It("Should handle very large size limits", func() {
			By("Testing with very large size limit")
			namespaceDelta := helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(1000), 10, 0, 0, 0)
			Expect(namespaceDelta).To(Equal(10))
		})

		It("Should handle boundary conditions", func() {
			By("Testing boundary condition: SizeLimit equals current namespace count")
			namespaceDelta := helpers.CalculateNamespaceQuantityDelta(utils.IntPtr(5), 2, 1, 1, 3)
			Expect(namespaceDelta).To(Equal(0))
		})
	})
})

var _ = Describe("Pool status updates and metrics", func() {
	Context("When pool status changes", func() {
		It("Should update status counters correctly", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			originalReady := pool.Status.Ready
			originalCreating := pool.Status.Creating
			originalReserved := pool.Status.Reserved

			By("Verifying status counts are non-negative")
			Expect(originalReady).To(BeNumerically(">=", 0))
			Expect(originalCreating).To(BeNumerically(">=", 0))
			Expect(originalReserved).To(BeNumerically(">=", 0))

			By("Verifying total count relationships")
			total := originalReady + originalCreating + originalReserved
			Expect(total).To(BeNumerically(">=", 0))
		})

		It("Should handle concurrent status updates", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating concurrent updates by rapid size changes")
			for i := 0; i < 3; i++ {
				pool.Spec.Size++
				err = k8sClient.Update(ctx, &pool)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}
		})
	})
})

var _ = Describe("Pool resource validation", func() {
	Context("When pool resources are created or updated", func() {
		It("Should handle pools with different configurations", func() {
			ctx := context.Background()

			By("Testing pools with different names exist")
			poolNames := []string{"default", "minimal", "limit"}
			for _, poolName := range poolNames {
				pool := crd.NamespacePool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: poolName}, &pool)
				Expect(err).NotTo(HaveOccurred())
				Expect(pool.Name).To(Equal(poolName))
			}
		})

		It("Should handle missing pools gracefully", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "non-existent-pool"}, &pool)
			Expect(k8serr.IsNotFound(err)).To(BeTrue())
		})
	})
})

var _ = Describe("Pool helper function validation", func() {
	Context("When using pool helper functions", func() {
		It("Should correctly identify when pool is at limit", func() {
			Expect(helpers.IsPoolAtLimit(5, 5)).To(BeTrue())
			Expect(helpers.IsPoolAtLimit(3, 5)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(7, 5)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(0, 0)).To(BeTrue())
		})

		It("Should handle edge cases in pool limit checking", func() {
			Expect(helpers.IsPoolAtLimit(-1, 5)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(5, -1)).To(BeFalse())
			Expect(helpers.IsPoolAtLimit(-1, -1)).To(BeTrue())
		})
	})
})
