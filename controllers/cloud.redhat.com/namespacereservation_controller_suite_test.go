package controllers

import (
	"context"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newReservation(resName string, duration string, requester string, pool string) *crd.NamespaceReservation {
	return &crd.NamespaceReservation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cloud.redhat.com/",
			Kind:       "NamespaceReservation",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: resName,
		},
		Spec: crd.NamespaceReservationSpec{
			Duration:  utils.StringPtr(duration),
			Requester: requester,
			Pool:      pool,
		},
	}
}

var _ = Describe("Reservation controller basic reservation", func() {
	const (
		timeout  = time.Second * 55
		duration = time.Second * 55
		interval = time.Millisecond * 250
	)

	Context("When creating a Reservation Resource", func() {
		It("Should assign a namespace to the reservation", func() {
			By("Updating the reservation")
			ctx := context.Background()
			resName := "test-frontend"

			reservation := newReservation(resName, "10h", "test-user-1", "default")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return false
				}
				if updatedReservation.Status.State == "active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle expired reservations", func() {
			By("Deleting the reservation")
			ctx := context.Background()
			resName := "short-reservation"

			reservation := newReservation(resName, "30s", "test-user-2", "default")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should ensure two namespaces are ready after all reservations are reconciled", func() {
			By("Creating more reservations then the pool size")
			ctx := context.Background()

			resName1 := "res-3"
			resName2 := "res-4"
			resName3 := "res-5"

			r1 := newReservation(resName1, "30m", "test-user-3", "minimal")
			r2 := newReservation(resName2, "30m", "test-user-4", "minimal")
			r3 := newReservation(resName3, "30m", "test-user-5", "minimal")

			Expect(k8sClient.Create(ctx, r1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r2)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r3)).Should(Succeed())

			updatedR1 := &crd.NamespaceReservation{}
			updatedR2 := &crd.NamespaceReservation{}
			updatedR3 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err1 := k8sClient.Get(ctx, types.NamespacedName{Name: resName1}, updatedR1)
				err2 := k8sClient.Get(ctx, types.NamespacedName{Name: resName2}, updatedR2)
				err3 := k8sClient.Get(ctx, types.NamespacedName{Name: resName3}, updatedR3)
				if errors.IsNotFound(err1) || err2 != nil || err3 != nil {
					return false
				}
				if updatedR2.Status.State == "active" &&
					updatedR3.Status.State == "active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Creating the number of ready namespaces as the pool size")
			pool := crd.NamespacePool{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "minimal"}, &pool)
				Expect(err).NotTo(HaveOccurred())

				return pool.Spec.Size == pool.Status.Ready
			}, timeout, interval).Should(BeTrue())
		})

	})
})
