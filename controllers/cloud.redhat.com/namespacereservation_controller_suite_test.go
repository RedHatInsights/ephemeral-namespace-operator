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
		Status: crd.NamespaceReservationStatus{
			Pool: pool,
		},
	}
}

var _ = Describe("Reservation controller basic reservation", func() {
	const (
		timeout  = time.Second * 4500
		duration = time.Second * 4500
		interval = time.Millisecond * 250
	)

	Context("When creating a Reservation Resource", func() {
		It("Should assign a namespace to the reservation", func() {
			By("Updating the reservation")
			ctx := context.Background()
			resName := "test-frontend"

			reservation := newReservation(resName, "10h", "psav", "default")

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

			reservation := newReservation(resName, "30s", "test-user", "default")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle waiting reservations", func() {
			By("Setting reservation state to waiting")
			ctx := context.Background()

			resName1 := "res-1"
			resName2 := "res-2"
			resName3 := "res-3"
			resName4 := "res-4"

			r1 := newReservation(resName1, "30s", "test-user-1", "minimal")
			r2 := newReservation(resName2, "10m", "test-user-2", "minimal")
			r3 := newReservation(resName3, "10m", "test-user-3", "minimal")
			r4 := newReservation(resName4, "10m", "test-user-4", "minimal")

			Expect(k8sClient.Create(ctx, r1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r2)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r3)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r4)).Should(Succeed())

			updatedR1 := &crd.NamespaceReservation{}
			updatedR2 := &crd.NamespaceReservation{}
			updatedR3 := &crd.NamespaceReservation{}
			updatedR4 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err1 := k8sClient.Get(ctx, types.NamespacedName{Name: resName1}, updatedR1)
				err2 := k8sClient.Get(ctx, types.NamespacedName{Name: resName2}, updatedR2)
				err3 := k8sClient.Get(ctx, types.NamespacedName{Name: resName3}, updatedR3)
				err4 := k8sClient.Get(ctx, types.NamespacedName{Name: resName4}, updatedR3)
				if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
					return false
				}
				if updatedR4.Status.State == "waiting" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err1 := k8sClient.Get(ctx, types.NamespacedName{Name: resName1}, updatedR1)
				err2 := k8sClient.Get(ctx, types.NamespacedName{Name: resName2}, updatedR2)
				err3 := k8sClient.Get(ctx, types.NamespacedName{Name: resName3}, updatedR3)
				err4 := k8sClient.Get(ctx, types.NamespacedName{Name: resName4}, updatedR4)
				if errors.IsNotFound(err1) || err2 != nil || err3 != nil || err4 != nil {
					return false
				}
				if updatedR2.Status.State == "active" &&
					updatedR3.Status.State == "active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
