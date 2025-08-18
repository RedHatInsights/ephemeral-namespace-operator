package controllers

import (
	"context"
	"fmt"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/ephemeral-namespace-operator/controllers/cloud.redhat.com/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	timeout  = time.Second * 30
	duration = time.Second * 30
	interval = time.Millisecond * 250
)

var _ = Describe("Reservation controller basic reservation", func() {
	Context("When creating a Reservation Resource", func() {
		It("Should assign a namespace to the reservation", func() {
			By("Updating the reservation")
			ctx := context.Background()
			resName := "test-res-01"

			reservation := helpers.NewReservation(resName, "10h", "test-user-01", "default", "")

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

		It("Should update the owner reference to be 'NamespaceReservation'", func() {
			ctx := context.Background()
			resName := "res-jksa43"

			reservation := helpers.NewReservation(resName, "10h", "test-user-jksa43", "default", "")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			reservedNamespace := core.Namespace{}
			for _, owner := range reservedNamespace.GetOwnerReferences() {
				Expect(owner.Kind == "NamespaceReservation").To(BeTrue())
			}
		})

		It("Should handle expired reservations", func() {
			By("Deleting the reservation")
			ctx := context.Background()
			resName := "test-user-02"

			reservation := helpers.NewReservation(resName, "15s", "test-user-02", "default", "")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				return k8serr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should ensure two namespaces are ready after all reservations are reconciled", func() {
			By("Creating more reservations then the pool size")
			ctx := context.Background()
			nsList := core.NamespaceList{}

			resName1 := "test-res-03"
			resName2 := "test-res-04"
			resName3 := "test-res-05"

			r1 := helpers.NewReservation(resName1, "30m", "test-user-03", "minimal", "")
			r2 := helpers.NewReservation(resName2, "30m", "test-user-04", "minimal", "")
			r3 := helpers.NewReservation(resName3, "30m", "test-user-05", "minimal", "")

			Expect(k8sClient.Create(ctx, r1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r2)).Should(Succeed())
			Expect(k8sClient.Create(ctx, r3)).Should(Succeed())

			updatedR1 := &crd.NamespaceReservation{}
			updatedR2 := &crd.NamespaceReservation{}
			updatedR3 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				nsCount := 0

				err1 := k8sClient.Get(ctx, types.NamespacedName{Name: resName1}, updatedR1)
				err2 := k8sClient.Get(ctx, types.NamespacedName{Name: resName2}, updatedR2)
				err3 := k8sClient.Get(ctx, types.NamespacedName{Name: resName3}, updatedR3)
				if k8serr.IsNotFound(err1) || err2 != nil || err3 != nil {
					return false
				}

				err := k8sClient.List(ctx, &nsList)
				for _, ns := range nsList.Items {
					if ns.Name == updatedR1.Status.Namespace {
						nsCount++
					} else if ns.Name == updatedR2.Status.Namespace {
						nsCount++
					} else if ns.Name == updatedR3.Status.Namespace {
						nsCount++
					}
				}
				Expect(err).NotTo(HaveOccurred())

				return nsCount == 3
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

var _ = Describe("Handle reservation without defined pool type", func() {
	Context("When creating a Reservation Resource without specifying the pool", func() {
		It("Should be able to set the pool to the 'default' pool", func() {
			ctx := context.Background()
			resName := "test-res-06"
			reservation := helpers.NewReservation(resName, "30s", "test-user-06", "", "")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())
			updatedR1 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedR1)
				if err != nil {
					return false
				}

				fmt.Println(reservation.Status.Expiration)

				return updatedR1.Status.Pool == "default"
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

var _ = Describe("Handle reservation without duration specified", func() {
	Context("When creating a Reservation Resource without specifying the duration", func() {
		It("Should be able to set the reservation duration to the default of 1 hour", func() {
			ctx := context.Background()
			resName := "test-res-00"
			reservation := helpers.NewReservation(resName, "", "test-user-06", "default", "")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())
			updatedR1 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedR1)
				Expect(err).NotTo(HaveOccurred())

				duration, err := parseDurationTime(*updatedR1.Spec.Duration)
				Expect(err).NotTo(HaveOccurred())

				return duration.String() == "1h0m0s"
			}, timeout, interval).Should(BeTrue())
		})
	})
})

var _ = Describe("Handle waiting reservations", func() {
	Context("When there are more reservations than ready namespaces", func() {
		It("Should create an excess of reservations to force some into a waiting state", func() {
			ctx := context.Background()

			resName1 := "test-res-07"
			resName2 := "test-res-08"
			resName3 := "test-res-09"
			resName4 := "test-res-10"
			resName5 := "test-res-11"

			res1 := helpers.NewReservation(resName1, "30m", "test-user-07", "minimal", "")
			res2 := helpers.NewReservation(resName2, "30m", "test-user-08", "minimal", "")
			res3 := helpers.NewReservation(resName3, "30m", "test-user-09", "minimal", "")
			res4 := helpers.NewReservation(resName4, "30m", "test-user-10", "minimal", "")
			res5 := helpers.NewReservation(resName5, "30m", "test-user-11", "minimal", "")

			Expect(k8sClient.Create(ctx, res1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res2)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res3)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res4)).Should(Succeed())
			Expect(k8sClient.Create(ctx, res5)).Should(Succeed())

			updatedRes5 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName5}, updatedRes5)
				Expect(err).NotTo(HaveOccurred())

				return updatedRes5.Status.State == "waiting"
			}, timeout, interval).Should(BeTrue())

		})
	})
})
