package controllers

import (
	"context"
	"fmt"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newClowdEnv(spec clowder.ClowdEnvironmentSpec, nsName string) clowder.ClowdEnvironment {
	env := clowder.ClowdEnvironment{
		Spec: spec,
	}
	env.SetName(fmt.Sprintf("env-%s", nsName))
	env.Spec.TargetNamespace = nsName

	return env
}

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

var _ = Describe("Basic creation of a Clowdenvironment", func() {
	Context("", func() {
		It("", func() {
			ctx := context.Background()
			minimalPool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "minimal"}, &minimalPool)
			Expect(err).NotTo(HaveOccurred())

			clowdEnv := newClowdEnv(minimalPool.Spec.ClowdEnvironment, "test-namespace-1")

			Expect(k8sClient.Create(ctx, clowdEnv)).Should(Succeed())

			updatedClowdEnv := clowder.ClowdEnvironment{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-namespace-1"}, &updatedClowdEnv)
				Expect(err).NotTo(HaveOccurred())
			})

		})
	})
})
