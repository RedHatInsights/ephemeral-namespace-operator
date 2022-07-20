package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
)

var _ = Describe("Clowdenvironment controller basic update", func() {
	const (
		timeout  = time.Second * 30
		duration = time.Second * 30
		interval = time.Millisecond * 250
	)

	Context("When a clowdenvironment is created", func() {
		It("Should update the namespace annotations when ready if owned by the pool", func() {
			By("Checking the clowdenvironment conditions")
			ctx := context.Background()
			nsList := core.NamespaceList{}

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
	})
})

var _ = Describe("Ensure successful creation of a clowdenvironment in a new namespace", func() {
	Context("When a new namespace is created", func() {
		It("Should successfully create the clowdenvironment", func() {
			ctx := context.Background()

			By("Ensuring creation of the clowdenvironemnt was successful")
			nsList := core.NamespaceList{}
			err := k8sClient.List(ctx, &nsList)
			Expect(err).NotTo(HaveOccurred())

			for _, ns := range nsList.Items {
				for _, owner := range ns.GetOwnerReferences() {
					if owner.Kind == "NamespacePool" {
						ready, _, err := GetClowdEnv(ctx, k8sClient, ns.Name)
						Expect(err).NotTo(HaveOccurred())

						Expect(ready).To(BeTrue())
					}
				}
			}
		})
	})
})
