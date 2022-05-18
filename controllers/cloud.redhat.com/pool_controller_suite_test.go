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
		timeout  = time.Second * 90
		duration = time.Second * 90
		interval = time.Millisecond * 25
	)

	Context("When a pool is reconciled", func() {
		It("Should reconcile the number of managed namespaces", func() {
			By("Comparing pool status to pool size")
			ctx := context.Background()
			pool := crd.NamespacePool{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default-pool"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Spec.Size == (pool.Status.Ready + pool.Status.Creating)
			}, timeout, interval).Should(BeTrue())

			By("Checking the total number of owned namespaces")
			nsList := core.NamespaceList{}

			Eventually(func() bool {
				err := k8sClient.List(ctx, &nsList)
				ownedCount := 0
				for _, ns := range nsList.Items {
					if isOwnedByPool(ctx, k8sClient, ns.Name) {
						ownedCount++
					}
				}
				Expect(err).NotTo(HaveOccurred())

				return ownedCount == pool.Spec.Size
			}, timeout, interval).Should(BeTrue())

			By("By creating new namespaces as needed")
			pool.Spec.Size++

			err := k8sClient.Update(ctx, &pool)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default-pool"}, &pool)
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

var _ = Describe("Ensure proper number of namespaces are in ready", func() {
	const (
		timeout  = time.Second * 90
		duration = time.Second * 90
		interval = time.Millisecond * 250
	)

	Context("When a namespace is deleted", func() {
		It("Should create a new namespace", func() {
			By("Seeing that the total of ready namespaces is less than the desired quantity")

		})
	})
})
