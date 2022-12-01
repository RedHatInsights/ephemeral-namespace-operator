package controllers

import (
	"context"
	"fmt"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	. "github.com/onsi/ginkgo"
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
			By("Comparing pool status to pool size")
			ctx := context.Background()
			pool := crd.NamespacePool{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Spec.Size == (pool.Status.Ready + pool.Status.Creating)
			}, timeout, interval).Should(BeTrue())

			By("Checking the total number of owned namespaces")
			nsList := core.NamespaceList{}

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

			By("Ensuring ready/creating namespace count equals the pool spec size")
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
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

			By("Creating new namespaces as needed")
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

		It("Should delete namespaces in an error state", func() {
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
					}
				}
			}
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

			res6 := helpers.NewReservation(resName6, "30m", "test-user-12", "limit")
			res7 := helpers.NewReservation(resName7, "30m", "test-user-13", "limit")

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
	})
})

var _ = Describe("Edge cases for creating namespaces with pool limit set", func() {
	Context("Ensure that calculating namespace creation or deletion is handled properly", func() {
		It("Should calculate namespace creation, deletion, or return 0 if no changes are needed", func() {
			// With a size limit of 8, ready at 0, and total namespaces at 2; create 2 ready namespaces
			By("Testing edge case => SizeLimit: 8, Size: 2, Ready: 0, Creating: 0, Reserved: 2")
			namespaceDelta := calculateNamespaceQuantityDelta(utils.IntPtr(8), 2, 0, 0, 2)

			Expect(namespaceDelta).To(Equal(2))

			// With a size limit of 3, ready at 0, and total namespaces at 2; create 1 more ready namespace
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 2")
			namespaceDelta = calculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 2)

			Expect(namespaceDelta).To(Equal(1))

			// Total namespaces equals pool limit, therefore no more namespaces are created
			By("Testing edge case => SizeLimit: 3, Size: 0, Ready: 0, Creating: 0, Reserved: 3")
			namespaceDelta = calculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 3)

			Expect(namespaceDelta).To(Equal(0))

			// No size limit on pool but we are at the max for ready namespace size then we don't create more namespaces
			By("Testing edge case => SizeLimit: nil, Size: 2, Ready: 2, Creating: 0, Reserved: 2")
			namespaceDelta = calculateNamespaceQuantityDelta(nil, 2, 2, 0, 2)

			Expect(namespaceDelta).To(Equal(0))

			// With no ready, but two creating, size is already hit and no more are created
			By("Testing edge case => SizeLimit: 5, Size: 2, Ready: 0, Creating: 2, Reserved: 2")
			namespaceDelta = calculateNamespaceQuantityDelta(utils.IntPtr(5), 2, 0, 2, 2)

			Expect(namespaceDelta).To(Equal(0))

			// If there are more namespaces then the size limit we reduce the ready namespace count from 2 to 1
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 2, Creating: 0, Reserved: 2")
			namespaceDelta = calculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 2, 0, 2)

			Expect(namespaceDelta).To(Equal(-1))

			// If 4 namespaces are reserved with pool size limit of 3, don't create more and don't remove any that are reserved
			// Since there are no ready namespaces to delete, this will not remove any namespaces
			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 4")
			namespaceDelta = calculateNamespaceQuantityDelta(utils.IntPtr(3), 2, 0, 0, 4)

			Expect(namespaceDelta).To(Equal(-1))
		})
	})
})
