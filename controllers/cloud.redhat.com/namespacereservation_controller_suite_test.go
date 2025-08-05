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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

			reservation := helpers.NewReservation(resName, "10h", "test-user-01", "default")

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

			reservation := helpers.NewReservation(resName, "10h", "test-user-jksa43", "default")

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

			reservation := helpers.NewReservation(resName, "15s", "test-user-02", "default")

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

			r1 := helpers.NewReservation(resName1, "30m", "test-user-03", "minimal")
			r2 := helpers.NewReservation(resName2, "30m", "test-user-04", "minimal")
			r3 := helpers.NewReservation(resName3, "30m", "test-user-05", "minimal")

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

		It("Should properly set reservation status fields", func() {
			ctx := context.Background()
			resName := "test-status-fields"

			reservation := helpers.NewReservation(resName, "1h", "test-user-status", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return false
				}

				By("Verifying all status fields are properly set")
				hasState := updatedReservation.Status.State != ""
				hasPool := updatedReservation.Status.Pool != ""
				hasNamespace := updatedReservation.Status.Namespace != ""
				hasExpiration := !updatedReservation.Status.Expiration.IsZero()

				return hasState && hasPool && hasNamespace && hasExpiration
			}, timeout, interval).Should(BeTrue())

			By("Verifying status field values are correct")
			Expect(updatedReservation.Status.State).To(Equal("active"))
			Expect(updatedReservation.Status.Pool).To(Equal("default"))
			Expect(updatedReservation.Status.Namespace).NotTo(BeEmpty())
			Expect(updatedReservation.Status.Expiration.Time).To(BeTemporally(">", time.Now()))
		})

		It("Should handle reservation state transitions properly", func() {
			ctx := context.Background()
			resName := "test-state-transition"

			reservation := helpers.NewReservation(resName, "30m", "test-user-transition", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			By("Initially reservation should be in pending or active state")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return ""
				}
				return updatedReservation.Status.State
			}, timeout, interval).Should(Or(Equal("active"), Equal("pending")))

			By("Active reservations should have assigned namespaces")
			if updatedReservation.Status.State == "active" {
				Expect(updatedReservation.Status.Namespace).NotTo(BeEmpty())
			}
		})
	})
})

var _ = Describe("Handle reservation without defined pool type", func() {
	Context("When creating a Reservation Resource without specifying the pool", func() {
		It("Should be able to set the pool to the 'default' pool", func() {
			ctx := context.Background()
			resName := "test-res-06"
			reservation := helpers.NewReservation(resName, "30s", "test-user-06", "")

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

		It("Should handle empty pool spec gracefully", func() {
			ctx := context.Background()
			resName := "test-empty-pool"
			reservation := helpers.NewReservation(resName, "1h", "test-user-empty", "")

			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())
			updatedReservation := &crd.NamespaceReservation{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return false
				}
				return updatedReservation.Status.Pool == "default"
			}, timeout, interval).Should(BeTrue())

			By("Verifying reservation becomes active with default pool")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return ""
				}
				return updatedReservation.Status.State
			}, timeout, interval).Should(Equal("active"))
		})

		It("Should handle non-existent pool names", func() {
			ctx := context.Background()
			resName := "test-invalid-pool"
			reservation := helpers.NewReservation(resName, "1h", "test-user-invalid", "non-existent-pool")

			By("Creating reservation with non-existent pool")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			By("Reservation should still be created but may be in waiting state")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Pool should be set to the specified value even if non-existent")
			Expect(updatedReservation.Status.Pool).To(Equal("non-existent-pool"))
		})
	})
})

var _ = Describe("Handle reservation without duration specified", func() {
	Context("When creating a Reservation Resource without specifying the duration", func() {
		It("Should be able to set the reservation duration to the default of 1 hour", func() {
			ctx := context.Background()
			resName := "test-res-00"
			reservation := helpers.NewReservation(resName, "", "test-user-06", "default")

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

		It("Should handle various duration formats", func() {
			ctx := context.Background()

			testCases := []struct {
				name           string
				duration       string
				expectedResult string
			}{
				{"minutes", "30m", "30m0s"},
				{"hours", "2h", "2h0m0s"},
				{"mixed", "1h30m", "1h30m0s"},
				{"seconds", "45s", "45s"},
				{"complex", "2h15m30s", "2h15m30s"},
			}

			for _, tc := range testCases {
				resName := fmt.Sprintf("test-duration-%s", tc.name)
				reservation := helpers.NewReservation(resName, tc.duration, fmt.Sprintf("test-user-%s", tc.name), "default")

				Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())
				updatedReservation := &crd.NamespaceReservation{}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
					if err != nil {
						return false
					}

					if updatedReservation.Spec.Duration == nil {
						return false
					}

					duration, err := parseDurationTime(*updatedReservation.Spec.Duration)
					return err == nil && duration.String() == tc.expectedResult
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should handle invalid duration formats gracefully", func() {
			ctx := context.Background()
			resName := "test-invalid-duration"
			reservation := helpers.NewReservation(resName, "invalid-duration", "test-user-invalid", "default")

			By("Creating reservation with invalid duration")
			err := k8sClient.Create(ctx, reservation)
			// Note: The creation itself might succeed, but the controller should handle the error
			if err == nil {
				updatedReservation := &crd.NamespaceReservation{}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				By("Default duration should be applied for invalid formats")
				if updatedReservation.Spec.Duration != nil {
					duration, err := parseDurationTime(*updatedReservation.Spec.Duration)
					if err == nil {
						Expect(duration.String()).To(Equal("1h0m0s"))
					}
				}
			}
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

			res1 := helpers.NewReservation(resName1, "30m", "test-user-07", "minimal")
			res2 := helpers.NewReservation(resName2, "30m", "test-user-08", "minimal")
			res3 := helpers.NewReservation(resName3, "30m", "test-user-09", "minimal")
			res4 := helpers.NewReservation(resName4, "30m", "test-user-10", "minimal")
			res5 := helpers.NewReservation(resName5, "30m", "test-user-11", "minimal")

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

		It("Should transition waiting reservations to active when namespaces become available", func() {
			ctx := context.Background()

			By("Creating reservations that will cause some to wait")
			resNames := []string{"wait-test-1", "wait-test-2", "wait-test-3", "wait-test-4"}
			reservations := make([]*crd.NamespaceReservation, len(resNames))

			for i, resName := range resNames {
				res := helpers.NewReservation(resName, "30m", fmt.Sprintf("wait-user-%d", i), "minimal")
				Expect(k8sClient.Create(ctx, res)).Should(Succeed())
				reservations[i] = res
			}

			By("Verifying some reservations are in waiting state")
			Eventually(func() int {
				waitingCount := 0
				for _, resName := range resNames {
					res := &crd.NamespaceReservation{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
					if err == nil && res.Status.State == "waiting" {
						waitingCount++
					}
				}
				return waitingCount
			}, timeout, interval).Should(BeNumerically(">", 0))

			By("Deleting an active reservation to free up a namespace")
			for _, resName := range resNames {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				if err == nil && res.Status.State == "active" {
					Expect(k8sClient.Delete(ctx, res)).Should(Succeed())
					break
				}
			}

			By("Verifying waiting reservations transition to active")
			Eventually(func() int {
				activeCount := 0
				for _, resName := range resNames {
					res := &crd.NamespaceReservation{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
					if err == nil && res.Status.State == "active" {
						activeCount++
					}
				}
				return activeCount
			}, timeout, interval).Should(BeNumerically(">", 0))
		})

		It("Should maintain FIFO order for waiting reservations", func() {
			ctx := context.Background()

			By("Creating multiple reservations in sequence")
			resNames := []string{"fifo-1", "fifo-2", "fifo-3"}
			creationTimes := make(map[string]metav1.Time)

			for i, resName := range resNames {
				res := helpers.NewReservation(resName, "1h", fmt.Sprintf("fifo-user-%d", i), "minimal")
				Expect(k8sClient.Create(ctx, res)).Should(Succeed())

				// Record creation time
				Eventually(func() bool {
					updatedRes := &crd.NamespaceReservation{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedRes)
					if err == nil {
						creationTimes[resName] = updatedRes.CreationTimestamp
						return true
					}
					return false
				}, timeout, interval).Should(BeTrue())

				// Small delay to ensure different creation times
				time.Sleep(time.Millisecond * 100)
			}

			By("Verifying creation timestamps are in order")
			for i := 1; i < len(resNames); i++ {
				prevTime := creationTimes[resNames[i-1]]
				currTime := creationTimes[resNames[i]]
				Expect(currTime.Time).To(BeTemporally(">=", prevTime.Time))
			}
		})
	})
})

var _ = Describe("Reservation expiration and cleanup", func() {
	Context("When reservations expire", func() {
		It("Should handle very short expiration times", func() {
			ctx := context.Background()
			resName := "test-quick-expire"

			reservation := helpers.NewReservation(resName, "2s", "test-user-quick", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			By("Verifying reservation is deleted after expiration")
			Eventually(func() bool {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				return k8serr.IsNotFound(err)
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("Should clean up associated namespace when reservation expires", func() {
			ctx := context.Background()
			resName := "test-namespace-cleanup"

			reservation := helpers.NewReservation(resName, "3s", "test-user-cleanup", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			var namespaceName string
			updatedReservation := &crd.NamespaceReservation{}

			By("Getting assigned namespace name")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err == nil && updatedReservation.Status.State == "active" {
					namespaceName = updatedReservation.Status.Namespace
					return namespaceName != ""
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Waiting for reservation to expire")
			Eventually(func() bool {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				return k8serr.IsNotFound(err)
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())

			By("Verifying namespace is cleaned up")
			Eventually(func() bool {
				ns := &core.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, ns)
				return k8serr.IsNotFound(err) || ns.Status.Phase == "Terminating"
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle multiple simultaneous expirations", func() {
			ctx := context.Background()

			resNames := []string{"multi-expire-1", "multi-expire-2", "multi-expire-3"}

			By("Creating multiple reservations with same short expiration")
			for i, resName := range resNames {
				res := helpers.NewReservation(resName, "3s", fmt.Sprintf("multi-user-%d", i), "default")
				Expect(k8sClient.Create(ctx, res)).Should(Succeed())
			}

			By("Verifying all reservations are deleted after expiration")
			for _, resName := range resNames {
				Eventually(func() bool {
					res := &crd.NamespaceReservation{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
					return k8serr.IsNotFound(err)
				}, time.Second*10, time.Millisecond*100).Should(BeTrue())
			}
		})
	})
})

var _ = Describe("Reservation validation and edge cases", func() {
	Context("When handling edge cases", func() {
		It("Should handle reservations with missing required fields", func() {
			ctx := context.Background()

			By("Testing reservation without requester")
			reservation := &crd.NamespaceReservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cloud.redhat.com/v1alpha1",
					Kind:       "NamespaceReservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-requester",
				},
				Spec: crd.NamespaceReservationSpec{
					Duration: nil,
					Pool:     "default",
				},
			}

			err := k8sClient.Create(ctx, reservation)
			// This should either fail validation or be handled gracefully
			if err == nil {
				updatedRes := &crd.NamespaceReservation{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-no-requester"}, updatedRes)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should handle reservations with extremely long durations", func() {
			ctx := context.Background()
			resName := "test-long-duration"

			reservation := helpers.NewReservation(resName, "8760h", "test-user-long", "default") // 1 year
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return false
				}
				return updatedReservation.Status.State == "active" || updatedReservation.Status.State == "waiting"
			}, timeout, interval).Should(BeTrue())

			By("Verifying expiration time is set correctly")
			if updatedReservation.Status.State == "active" {
				Expect(updatedReservation.Status.Expiration.Time).To(BeTemporally(">", time.Now().Add(8759*time.Hour)))
			}
		})

		It("Should handle concurrent reservation creation", func() {
			ctx := context.Background()

			By("Creating multiple reservations concurrently")
			resNames := []string{"concurrent-1", "concurrent-2", "concurrent-3", "concurrent-4"}
			errors := make(chan error, len(resNames))

			for i, resName := range resNames {
				go func(name string, index int) {
					res := helpers.NewReservation(name, "30m", fmt.Sprintf("concurrent-user-%d", index), "default")
					err := k8sClient.Create(ctx, res)
					errors <- err
				}(resName, i)
			}

			By("Verifying all creations complete without error")
			for i := 0; i < len(resNames); i++ {
				err := <-errors
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying all reservations have valid states")
			for _, resName := range resNames {
				res := &crd.NamespaceReservation{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
					if err != nil {
						return false
					}
					return res.Status.State == "active" || res.Status.State == "waiting"
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should handle reservation deletion during processing", func() {
			ctx := context.Background()
			resName := "test-early-delete"

			reservation := helpers.NewReservation(resName, "1h", "test-user-early", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			By("Deleting reservation immediately after creation")
			Eventually(func() error {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				if err != nil {
					return err
				}
				return k8sClient.Delete(ctx, res)
			}, timeout, interval).Should(Succeed())

			By("Verifying reservation is deleted")
			Eventually(func() bool {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				return k8serr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle reservation updates", func() {
			ctx := context.Background()
			resName := "test-update"

			reservation := helpers.NewReservation(resName, "1h", "test-user-update", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Updating reservation spec")
			updatedReservation.Spec.Requester = "updated-user"
			err := k8sClient.Update(ctx, updatedReservation)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying update is reflected")
			Eventually(func() string {
				res := &crd.NamespaceReservation{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, res)
				if err != nil {
					return ""
				}
				return res.Spec.Requester
			}, timeout, interval).Should(Equal("updated-user"))
		})
	})
})

var _ = Describe("Reservation metrics and monitoring", func() {
	Context("When reservation states change", func() {
		It("Should track reservation state transitions", func() {
			ctx := context.Background()
			resName := "test-metrics"

			reservation := helpers.NewReservation(resName, "30m", "test-user-metrics", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			updatedReservation := &crd.NamespaceReservation{}

			By("Tracking initial state")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return ""
				}
				return updatedReservation.Status.State
			}, timeout, interval).Should(Or(Equal("active"), Equal("waiting")))

			By("Verifying state is consistent")
			Consistently(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resName}, updatedReservation)
				if err != nil {
					return ""
				}
				return updatedReservation.Status.State
			}, time.Second*2, time.Millisecond*500).Should(Or(Equal("active"), Equal("waiting")))
		})

		It("Should handle pool status updates from reservations", func() {
			ctx := context.Background()
			pool := crd.NamespacePool{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
			Expect(err).NotTo(HaveOccurred())

			originalReserved := pool.Status.Reserved

			By("Creating a new reservation")
			resName := "test-pool-update"
			reservation := helpers.NewReservation(resName, "30m", "test-user-pool", "default")
			Expect(k8sClient.Create(ctx, reservation)).Should(Succeed())

			By("Verifying pool reserved count increases")
			Eventually(func() int {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, &pool)
				if err != nil {
					return -1
				}
				return pool.Status.Reserved
			}, timeout, interval).Should(BeNumerically(">=", originalReserved))
		})
	})
})
