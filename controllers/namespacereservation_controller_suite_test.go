package controllers

import (
	"context"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Namespace controller basic reservation", func() {
	const (
		ReservationName = "test-frontend"

		timeout  = time.Second * 90
		duration = time.Second * 90
		interval = time.Millisecond * 250
	)

	Context("When creating a Reservation Resource", func() {
		It("Should setup the namespace properly", func() {
			By("Creating a namespace/project")
			ctx := context.Background()

			reservation := &crd.NamespaceReservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cloud.redhat.com/",
					Kind:       "NamespaceReservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: ReservationName,
				},
				Spec: crd.NamespaceReservationSpec{
					Duration:  utils.StringPtr("10h"),
					Requester: "psav",
				},
			}

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: ReservationName}, updatedReservation)
				if err != nil {
					return false
				}
				if updatedReservation.Status.State == "active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
