package controllers

import (
	"context"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
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

			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Status.Ready + pool.Status.Creating
			}, timeout, interval).Should(Equal(1))

			By("Creating new namespaces as needed")
			pool.Spec.Size++

			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Spec.Size == pool.Status.Ready
			}, timeout, interval).Should(BeTrue())

			pool.Spec.Size--

			err = k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())
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

			err = UpdateAnnotations(ctx, k8sClient, map[string]string{"env-status": "error"}, ownedNs.Name)
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
