package controllers

import (
	"context"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

			By("Creating new namespaces as needed")
			pool.Spec.Size++

			err := k8sClient.Update(ctx, &pool)
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

var _ = Describe("Handle deletion of the prometheus operator when namespace is deleted", func() {
	Context("When an ephemeral namespace is deleted", func() {
		It("Should ensure deletion of the prometheus operator associated with the deleted namespace", func() {
			ctx := context.Background()
			nsList := core.NamespaceList{}

			By("Choosing a namespace to be associated with prometheus operator")
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

			By("Creating a prometheus operator for the newly reserved namespace")
			prometheusOperator := unstructured.Unstructured{}

			gvk := schema.GroupVersionKind{
				Group:   "operators.coreos.com",
				Version: "v1",
				Kind:    "Operator",
			}
			prometheusOperator.SetGroupVersionKind(gvk)
			prometheusOperator.SetName(GetPrometheusOperatorName(ownedNs.Name))
			Expect(ownedNs.Name).NotTo(BeEmpty())

			err = k8sClient.Create(ctx, &prometheusOperator)
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the namespace")

			err = UpdateAnnotations(ctx, k8sClient, map[string]string{"env-status": "error"}, ownedNs.Name)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: ownedNs.Name}, &ownedNs)
				if k8serr.IsNotFound(err) || ownedNs.Status.Phase == "Terminating" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Checking that the prometheus operator was deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: GetPrometheusOperatorName(ownedNs.Name)}, &prometheusOperator)

				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
