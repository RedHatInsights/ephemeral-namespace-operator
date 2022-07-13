package controllers

import (
	"context"
	"fmt"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	timeout  = time.Second * 30
	duration = time.Second * 30
	interval = time.Millisecond * 250
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
	Context("When creating a Reservation Resource", func() {
		It("Should assign a namespace to the reservation", func() {
			By("Updating the reservation")
			ctx := context.Background()
			resName := "test-res-01"

			reservation := newReservation(resName, "10h", "test-user-01", "default")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedRes1 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedRes1)
				if err != nil {
					return false
				}
				if updatedRes1.Status.State == "active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle expired reservations", func() {
			By("Deleting the reservation")
			ctx := context.Background()
			resName := "test-user-02"

			reservation := newReservation(resName, "15s", "test-user-02", "default")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedRes2 := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedRes2)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should ensure two namespaces are ready after all reservations are reconciled", func() {
			By("Creating more reservations then the pool size")
			ctx := context.Background()
			nsList := core.NamespaceList{}

			resName1 := "test-res-03"
			resName2 := "test-res-04"
			resName3 := "test-res-05"

			r1 := newReservation(resName1, "30m", "test-user-03", "minimal")
			r2 := newReservation(resName2, "30m", "test-user-04", "minimal")
			r3 := newReservation(resName3, "30m", "test-user-05", "minimal")

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
				if errors.IsNotFound(err1) || err2 != nil || err3 != nil {
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
			reservation := newReservation(resName, "30s", "test-user-06", "")

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

var _ = Describe("Handle waiting reservations", func() {
	Context("When there are more reservations than ready namespaces", func() {
		It("Should create a new reservation", func() {
			ctx := context.Background()

			resName1 := "test-res-07"
			resName2 := "test-res-08"
			resName3 := "test-res-09"
			resName4 := "test-res-10"
			resName5 := "test-res-11"

			res1 := newReservation(resName1, "30m", "test-user-07", "minimal")
			res2 := newReservation(resName2, "30m", "test-user-08", "minimal")
			res3 := newReservation(resName3, "30m", "test-user-09", "minimal")
			res4 := newReservation(resName4, "30m", "test-user-10", "minimal")
			res5 := newReservation(resName5, "30m", "test-user-11", "minimal")

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

var _ = Describe("Handle deletion of the prometheus operator when reservation is deleted", func() {
	Context("When an reservation is deleted", func() {
		It("Should ensure deletion of the prometheus operator associated with the deleted namespace", func() {
			ctx := context.Background()

			By("Creating a new reservation")
			resName6 := "test-res-12"

			res6 := newReservation(resName6, "35s", "test-user-12", "default")

			Expect(k8sClient.Create(ctx, res6)).Should(Succeed())

			updatedRes6 := &crd.NamespaceReservation{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName6}, updatedRes6)
				Expect(err).NotTo(HaveOccurred())

				return updatedRes6.Status.Namespace
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Creating a subscription for the prometheus operator for the newly reserved namespace")
			subscription := unstructured.Unstructured{}

			gvkSubscription := schema.GroupVersionKind{
				Group:   "operators.coreos.com",
				Version: "v1alpha1",
				Kind:    "Subscription",
			}

			subscription.SetGroupVersionKind(gvkSubscription)
			subscription.SetName("prometheus")
			subscription.SetNamespace(updatedRes6.Status.Namespace)
			Expect(updatedRes6.Name).NotTo(BeEmpty())

			err := k8sClient.Create(ctx, &subscription)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a prometheus operator for the newly reserved namespace")
			prometheusOperator := unstructured.Unstructured{}

			gvkPrometheus := schema.GroupVersionKind{
				Group:   "operators.coreos.com",
				Version: "v1",
				Kind:    "Operator",
			}
			prometheusOperator.SetGroupVersionKind(gvkPrometheus)
			prometheusOperator.SetName(GetPrometheusOperatorName(updatedRes6.Status.Namespace))
			prometheusOperator.SetNamespace(updatedRes6.Status.Namespace)
			Expect(updatedRes6.Name).NotTo(BeEmpty())

			err = k8sClient.Create(ctx, &prometheusOperator)
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the reservation")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: updatedRes6.Status.Namespace}, updatedRes6)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Checking that the subscription for the prometheus operator was deleted")

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: res6.Status.Namespace, Name: "prometheus"}, &subscription)

				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("Checking that the prometheus operator was deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: GetPrometheusOperatorName(res6.Status.Namespace)}, &prometheusOperator)

				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
