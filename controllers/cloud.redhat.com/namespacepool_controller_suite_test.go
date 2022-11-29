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
	Context("", func() {
		It("", func() {
			By("Testing edge case => SizeLimit: 8, Size: 2, Ready: 0, Creating: 0, Reserved: 2")
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "test"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			resName1 := "test-res-12"
			resName2 := "test-res-13"

			res1 := helpers.NewReservation(resName1, "30m", "test-user-07", "test")
			res2 := helpers.NewReservation(resName2, "30m", "test-user-08", "test")

			Expect(k8sClient.Create(ctx, res1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res2)).Should(Succeed())

			pool.Spec.SizeLimit = utils.IntPtr(8)

			Expect(k8sClient.Update(ctx, &pool)).To(BeNil())

			expected := fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", 2, 0, 2)

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				ready := pool.Status.Ready
				creating := pool.Status.Creating
				reserved := pool.Status.Reserved

				return fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", ready, creating, reserved)
			}, timeout, interval).Should(Equal(expected))

			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 2")
			pool.Spec.SizeLimit = utils.IntPtr(3)

			Expect(k8sClient.Update(ctx, &pool)).To(BeNil())

			expected = fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", 1, 0, 2)

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				ready := pool.Status.Ready
				creating := pool.Status.Creating
				reserved := pool.Status.Reserved

				return fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", ready, creating, reserved)
			}, timeout, interval).Should(Equal(expected))

			By("Testing edge case => SizeLimit: 3, Size: 2, Ready: 0, Creating: 0, Reserved: 3")
			resName3 := "test-res-14"

			res3 := helpers.NewReservation(resName3, "30m", "test-user-09", "test")

			Expect(k8sClient.Create(ctx, res3)).Should(Succeed())

			expected = fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", 0, 0, 3)

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				ready := pool.Status.Ready
				creating := pool.Status.Creating
				reserved := pool.Status.Reserved

				return fmt.Sprintf("Ready: [%d], Creating: [%d], Reserved: [%d]", ready, creating, reserved)
			}, timeout, interval).Should(Equal(expected))
		})
	})
})
