package helpers

import (
	"context"
	"testing"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helpers Suite")
}

var _ = Describe("Helper Functions", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		logger     logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(core.AddToScheme(scheme)).To(Succeed())
		Expect(clowder.AddToScheme(scheme)).To(Succeed())
		Expect(crd.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		logger = log.Log.WithName("test-helpers")
	})

	Describe("Pool Helper Functions", func() {
		Describe("IsPoolAtLimit", func() {
			It("should return true when current size equals limit", func() {
				result := IsPoolAtLimit(5, 5)
				Expect(result).To(BeTrue())
			})

			It("should return false when current size is less than limit", func() {
				result := IsPoolAtLimit(3, 5)
				Expect(result).To(BeFalse())
			})

			It("should return false when current size is greater than limit", func() {
				result := IsPoolAtLimit(7, 5)
				Expect(result).To(BeFalse())
			})

			It("should handle zero values correctly", func() {
				result := IsPoolAtLimit(0, 0)
				Expect(result).To(BeTrue())
			})
		})

		Describe("CalculateNamespaceQuantityDelta", func() {
			Context("when poolSizeLimit is nil (no limit)", func() {
				It("should calculate delta based on size and current queue", func() {
					delta := CalculateNamespaceQuantityDelta(nil, 5, 2, 1, 1)
					// size (5) - (ready (2) + creating (1)) = 2
					Expect(delta).To(Equal(2))
				})

				It("should return negative delta when queue exceeds size", func() {
					delta := CalculateNamespaceQuantityDelta(nil, 3, 2, 2, 1)
					// size (3) - (ready (2) + creating (2)) = -1
					Expect(delta).To(Equal(-1))
				})

				It("should return zero when queue equals size", func() {
					delta := CalculateNamespaceQuantityDelta(nil, 4, 2, 2, 1)
					// size (4) - (ready (2) + creating (2)) = 0
					Expect(delta).To(Equal(0))
				})
			})

			Context("when poolSizeLimit is set", func() {
				It("should respect size limit when calculating delta", func() {
					sizeLimit := 8
					delta := CalculateNamespaceQuantityDelta(&sizeLimit, 5, 2, 1, 2)
					// Current total: ready (2) + creating (1) + reserved (2) = 5
					// Current queue: ready (2) + creating (1) = 3
					// Size needed: 5 - 3 = 2
					// Limit allows: 8 - 5 = 3
					// Min(2, 3) = 2
					Expect(delta).To(Equal(2))
				})

				It("should limit creation when approaching size limit", func() {
					sizeLimit := 6
					delta := CalculateNamespaceQuantityDelta(&sizeLimit, 5, 1, 1, 3)
					// Current total: ready (1) + creating (1) + reserved (3) = 5
					// Current queue: ready (1) + creating (1) = 2
					// Size needed: 5 - 2 = 3
					// Limit allows: 6 - 5 = 1
					// Min(3, 1) = 1
					Expect(delta).To(Equal(1))
				})

				It("should return zero when at size limit", func() {
					sizeLimit := 5
					delta := CalculateNamespaceQuantityDelta(&sizeLimit, 3, 1, 1, 3)
					// Current total: ready (1) + creating (1) + reserved (3) = 5
					// Current queue: ready (1) + creating (1) = 2
					// Size needed: 3 - 2 = 1
					// Limit allows: 5 - 5 = 0
					// Min(1, 0) = 0
					Expect(delta).To(Equal(0))
				})

				It("should return negative delta when over limit", func() {
					sizeLimit := 4
					delta := CalculateNamespaceQuantityDelta(&sizeLimit, 3, 2, 1, 2)
					// Current total: ready (2) + creating (1) + reserved (2) = 5
					// Current queue: ready (2) + creating (1) = 3
					// Size needed: 3 - 3 = 0
					// Limit allows: 4 - 5 = -1
					// Min(0, -1) = -1
					Expect(delta).To(Equal(-1))
				})
			})
		})
	})

	Describe("Types Helper Functions", func() {
		Describe("CreateInitialAnnotations", func() {
			It("should create correct initial annotations", func() {
				annotations := CreateInitialAnnotations()

				Expect(annotations).To(HaveLen(2))
				Expect(annotations[AnnotationEnvStatus]).To(Equal(EnvStatusCreating))
				Expect(annotations[AnnotationReserved]).To(Equal(FalseValue))
			})

			It("should return a new map each time", func() {
				annotations1 := CreateInitialAnnotations()
				annotations2 := CreateInitialAnnotations()

				// Modify one map
				annotations1["test"] = "value"

				// The other should be unaffected
				Expect(annotations2).ToNot(HaveKey("test"))
			})
		})

		Describe("CreateInitialLabels", func() {
			It("should create correct initial labels with pool name", func() {
				poolName := "test-pool"
				labels := CreateInitialLabels(poolName)

				Expect(labels).To(HaveLen(2))
				Expect(labels[LabelOperatorNS]).To(Equal(TrueValue))
				Expect(labels[LabelPool]).To(Equal(poolName))
			})

			It("should handle empty pool name", func() {
				labels := CreateInitialLabels("")

				Expect(labels).To(HaveLen(2))
				Expect(labels[LabelOperatorNS]).To(Equal(TrueValue))
				Expect(labels[LabelPool]).To(Equal(""))
			})
		})

		Describe("CustomAnnotation", func() {
			It("should convert to map correctly", func() {
				annotation := CustomAnnotation{
					Annotation: "test-annotation",
					Value:      "test-value",
				}

				result := annotation.ToMap()

				Expect(result).To(HaveLen(1))
				Expect(result["test-annotation"]).To(Equal("test-value"))
			})

			It("should handle empty values", func() {
				annotation := CustomAnnotation{
					Annotation: "",
					Value:      "",
				}

				result := annotation.ToMap()

				Expect(result).To(HaveLen(1))
				Expect(result[""]).To(Equal(""))
			})
		})

		Describe("CustomLabel", func() {
			It("should convert to map correctly", func() {
				label := CustomLabel{
					Label: "test-label",
					Value: "test-value",
				}

				result := label.ToMap()

				Expect(result).To(HaveLen(1))
				Expect(result["test-label"]).To(Equal("test-value"))
			})
		})

		Describe("Predefined Annotations", func() {
			It("should have correct values for AnnotationEnvReady", func() {
				result := AnnotationEnvReady.ToMap()
				Expect(result[AnnotationEnvStatus]).To(Equal(EnvStatusReady))
			})

			It("should have correct values for AnnotationEnvCreating", func() {
				result := AnnotationEnvCreating.ToMap()
				Expect(result[AnnotationEnvStatus]).To(Equal(EnvStatusCreating))
			})

			It("should have correct values for AnnotationEnvError", func() {
				result := AnnotationEnvError.ToMap()
				Expect(result[AnnotationEnvStatus]).To(Equal(EnvStatusError))
			})

			It("should have correct values for AnnotationEnvDeleting", func() {
				result := AnnotationEnvDeleting.ToMap()
				Expect(result[AnnotationEnvStatus]).To(Equal(EnvStatusDeleting))
			})
		})
	})

	Describe("ClowdEnvs Helper Functions", func() {
		Describe("CreateClowdEnv", func() {
			var (
				namespace *core.Namespace
				spec      clowder.ClowdEnvironmentSpec
			)

			BeforeEach(func() {
				namespace = &core.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-namespace",
						UID:  "test-uid",
					},
				}
				namespace.SetGroupVersionKind(core.SchemeGroupVersion.WithKind("Namespace"))

				spec = clowder.ClowdEnvironmentSpec{
					TargetNamespace: "test-namespace",
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(namespace).
					Build()
			})

			It("should create ClowdEnvironment successfully", func() {
				err := CreateClowdEnv(ctx, fakeClient, spec, "test-namespace")
				Expect(err).ToNot(HaveOccurred())

				// Verify the ClowdEnvironment was created
				env := &clowder.ClowdEnvironment{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      "env-test-namespace",
					Namespace: "",
				}, env)
				Expect(err).ToNot(HaveOccurred())
				Expect(env.Spec.TargetNamespace).To(Equal("test-namespace"))
				Expect(env.GetOwnerReferences()).To(HaveLen(1))
				Expect(env.GetOwnerReferences()[0].Name).To(Equal("test-namespace"))
			})

			It("should return error when namespace doesn't exist", func() {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

				err := CreateClowdEnv(ctx, fakeClient, spec, "nonexistent-namespace")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not retrieve namespace"))
			})
		})

		Describe("GetClowdEnv", func() {
			It("should return error when ClowdEnvironment doesn't exist", func() {
				ready, env, err := GetClowdEnv(ctx, fakeClient, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(ready).To(BeFalse())
				Expect(env).To(BeNil())
			})

			It("should return ClowdEnvironment when it exists", func() {
				clowdEnv := &clowder.ClowdEnvironment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "env-test-namespace",
						Namespace: "test-namespace",
					},
					Status: clowder.ClowdEnvironmentStatus{
						Conditions: []clusterv1.Condition{
							{
								Type:   "ReconciliationSuccessful",
								Status: "True",
							},
							{
								Type:   "DeploymentsReady",
								Status: "True",
							},
						},
					},
				}

				Expect(fakeClient.Create(ctx, clowdEnv)).To(Succeed())

				ready, env, err := GetClowdEnv(ctx, fakeClient, "test-namespace")
				Expect(err).ToNot(HaveOccurred())
				Expect(ready).To(BeTrue())
				Expect(env).ToNot(BeNil())
				Expect(env.Name).To(Equal("env-test-namespace"))
			})
		})

		Describe("VerifyClowdEnvReady", func() {
			It("should return true when all conditions are met", func() {
				env := clowder.ClowdEnvironment{
					Status: clowder.ClowdEnvironmentStatus{
						Conditions: []clusterv1.Condition{
							{
								Type:   "ReconciliationSuccessful",
								Status: "True",
							},
							{
								Type:   "DeploymentsReady",
								Status: "True",
							},
						},
					},
				}

				ready := VerifyClowdEnvReady(env)
				Expect(ready).To(BeTrue())
			})

			It("should return false when ReconciliationSuccessful is false", func() {
				env := clowder.ClowdEnvironment{
					Status: clowder.ClowdEnvironmentStatus{
						Conditions: []clusterv1.Condition{
							{
								Type:   "ReconciliationSuccessful",
								Status: "False",
							},
							{
								Type:   "DeploymentsReady",
								Status: "True",
							},
						},
					},
				}

				ready := VerifyClowdEnvReady(env)
				Expect(ready).To(BeFalse())
			})

			It("should return false when DeploymentsReady is false", func() {
				env := clowder.ClowdEnvironment{
					Status: clowder.ClowdEnvironmentStatus{
						Conditions: []clusterv1.Condition{
							{
								Type:   "ReconciliationSuccessful",
								Status: "True",
							},
							{
								Type:   "DeploymentsReady",
								Status: "False",
							},
						},
					},
				}

				ready := VerifyClowdEnvReady(env)
				Expect(ready).To(BeFalse())
			})

			It("should return false when conditions are missing", func() {
				env := clowder.ClowdEnvironment{
					Status: clowder.ClowdEnvironmentStatus{
						Conditions: []clusterv1.Condition{},
					},
				}

				ready := VerifyClowdEnvReady(env)
				Expect(ready).To(BeFalse())
			})

			It("should return false when hostname is missing in local web mode", func() {
				env := clowder.ClowdEnvironment{
					Spec: clowder.ClowdEnvironmentSpec{
						Providers: clowder.ProvidersConfig{
							Web: clowder.WebConfig{
								Mode: "local",
							},
						},
					},
					Status: clowder.ClowdEnvironmentStatus{
						Hostname: "",
						Conditions: []clusterv1.Condition{
							{
								Type:   "ReconciliationSuccessful",
								Status: "True",
							},
							{
								Type:   "DeploymentsReady",
								Status: "True",
							},
						},
					},
				}

				ready := VerifyClowdEnvReady(env)
				Expect(ready).To(BeFalse())
			})

			It("should return true when hostname is present in local web mode", func() {
				env := clowder.ClowdEnvironment{
					Spec: clowder.ClowdEnvironmentSpec{
						Providers: clowder.ProvidersConfig{
							Web: clowder.WebConfig{
								Mode: "local",
							},
						},
					},
					Status: clowder.ClowdEnvironmentStatus{
						Hostname: "test.example.com",
						Conditions: []clusterv1.Condition{
							{
								Type:   "ReconciliationSuccessful",
								Status: "True",
							},
							{
								Type:   "DeploymentsReady",
								Status: "True",
							},
						},
					},
				}

				ready := VerifyClowdEnvReady(env)
				Expect(ready).To(BeTrue())
			})
		})
	})

	Describe("Reservation Helper Functions", func() {
		Describe("NewReservation", func() {
			It("should create a reservation with correct fields", func() {
				res := NewReservation("test-res", "1h", "test-user", "test-team", "test-pool")

				Expect(res.Name).To(Equal("test-res"))
				Expect(res.Spec.Duration).To(Equal(utils.StringPtr("1h")))
				Expect(res.Spec.Requester).To(Equal("test-user"))
				Expect(res.Spec.Team).To(Equal("test-team"))
				Expect(res.Spec.Pool).To(Equal("test-pool"))
				Expect(res.Kind).To(Equal("NamespaceReservation"))
			})

			It("should handle empty values", func() {
				res := NewReservation("", "", "", "", "")

				Expect(res.Name).To(Equal(""))
				Expect(res.Spec.Duration).To(Equal(utils.StringPtr("")))
				Expect(res.Spec.Requester).To(Equal(""))
				Expect(res.Spec.Team).To(Equal(""))
				Expect(res.Spec.Pool).To(Equal(""))
			})
		})

		Describe("findTeamByName", func() {
			It("should find team by name", func() {
				teams := []crd.Team{
					{Name: "team1", Secrets: []crd.SecretsData{{Name: "secret1"}}},
					{Name: "team2", Secrets: []crd.SecretsData{{Name: "secret2"}}},
				}

				team := findTeamByName(teams, "team2")
				Expect(team).ToNot(BeNil())
				Expect(team.Name).To(Equal("team2"))
				Expect(team.Secrets[0].Name).To(Equal("secret2"))
			})

			It("should return nil when team not found", func() {
				teams := []crd.Team{
					{Name: "team1", Secrets: []crd.SecretsData{{Name: "secret1"}}},
				}

				team := findTeamByName(teams, "nonexistent")
				Expect(team).To(BeNil())
			})

			It("should handle empty teams slice", func() {
				teams := []crd.Team{}

				team := findTeamByName(teams, "any-team")
				Expect(team).To(BeNil())
			})
		})

		Describe("findSecretByName", func() {
			It("should find secret by name", func() {
				secrets := []core.Secret{
					{ObjectMeta: metav1.ObjectMeta{Name: "secret1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "secret2"}},
				}

				secret := findSecretByName(secrets, "secret2")
				Expect(secret).ToNot(BeNil())
				Expect(secret.Name).To(Equal("secret2"))
			})

			It("should return nil when secret not found", func() {
				secrets := []core.Secret{
					{ObjectMeta: metav1.ObjectMeta{Name: "secret1"}},
				}

				secret := findSecretByName(secrets, "nonexistent")
				Expect(secret).To(BeNil())
			})

			It("should handle empty secrets slice", func() {
				secrets := []core.Secret{}

				secret := findSecretByName(secrets, "any-secret")
				Expect(secret).To(BeNil())
			})
		})

		Describe("copySecretToNamespace", func() {
			It("should copy secret with same name when DestName is empty", func() {
				secret := &core.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret",
						Namespace: "source-ns",
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				}

				teamSecret := crd.SecretsData{
					Name: "source-secret",
				}

				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				err := copySecretToNamespace(ctx, fakeClient, secret, teamSecret, "target-ns")
				Expect(err).ToNot(HaveOccurred())

				// Verify the secret was copied
				copiedSecret := &core.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      "source-secret",
					Namespace: "target-ns",
				}, copiedSecret)
				Expect(err).ToNot(HaveOccurred())
				Expect(copiedSecret.Data["key1"]).To(Equal([]byte("value1")))
			})

			It("should copy secret with different name when DestName is provided", func() {
				secret := &core.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret",
						Namespace: "source-ns",
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				}

				teamSecret := crd.SecretsData{
					Name:     "source-secret",
					DestName: "dest-secret",
				}

				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				err := copySecretToNamespace(ctx, fakeClient, secret, teamSecret, "target-ns")
				Expect(err).ToNot(HaveOccurred())

				// Verify the secret was copied with the destination name
				copiedSecret := &core.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      "dest-secret",
					Namespace: "target-ns",
				}, copiedSecret)
				Expect(err).ToNot(HaveOccurred())
				Expect(copiedSecret.Data["key1"]).To(Equal([]byte("value1")))
			})
		})

		Describe("copyTeamSecrets", func() {
			It("should copy all team secrets", func() {
				secrets := []core.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret1",
							Namespace: "source-ns",
						},
						Data: map[string][]byte{"key1": []byte("value1")},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret2",
							Namespace: "source-ns",
						},
						Data: map[string][]byte{"key2": []byte("value2")},
					},
				}

				teamSecrets := []crd.SecretsData{
					{Name: "secret1"},
					{Name: "secret2"},
				}

				// Create source secrets
				for _, secret := range secrets {
					Expect(fakeClient.Create(ctx, &secret)).To(Succeed())
				}

				err := copyTeamSecrets(ctx, fakeClient, secrets, teamSecrets, "target-ns", logger)
				Expect(err).ToNot(HaveOccurred())

				// Verify both secrets were copied
				for _, teamSecret := range teamSecrets {
					copiedSecret := &core.Secret{}
					err = fakeClient.Get(ctx, types.NamespacedName{
						Name:      teamSecret.Name,
						Namespace: "target-ns",
					}, copiedSecret)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("should skip missing secrets gracefully", func() {
				secrets := []core.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret1",
							Namespace: "source-ns",
						},
						Data: map[string][]byte{"key1": []byte("value1")},
					},
				}

				teamSecrets := []crd.SecretsData{
					{Name: "secret1"},
					{Name: "missing-secret"}, // This secret doesn't exist
				}

				// Create only one secret
				Expect(fakeClient.Create(ctx, &secrets[0])).To(Succeed())

				err := copyTeamSecrets(ctx, fakeClient, secrets, teamSecrets, "target-ns", logger)
				Expect(err).ToNot(HaveOccurred())

				// Verify the existing secret was copied
				copiedSecret := &core.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      "secret1",
					Namespace: "target-ns",
				}, copiedSecret)
				Expect(err).ToNot(HaveOccurred())

				// Verify the missing secret was not copied (should not exist)
				missingSecret := &core.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      "missing-secret",
					Namespace: "target-ns",
				}, missingSecret)
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("CopyReservationSecrets", func() {
			It("should return nil when team is not found in pool configuration", func() {
				// Create a namespace pool without the requested team
				pool := &crd.NamespacePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-development",
						Namespace: NamespaceHcmAi,
					},
					Spec: crd.NamespacePoolSpec{
						Teams: []crd.Team{
							{Name: "other-team", Secrets: []crd.SecretsData{{Name: "secret1"}}},
						},
					},
				}

				reservation := &crd.NamespaceReservation{
					Spec: crd.NamespaceReservationSpec{
						Team: "nonexistent-team",
					},
				}

				Expect(fakeClient.Create(ctx, pool)).To(Succeed())

				err := CopyReservationSecrets(ctx, fakeClient, "target-ns", reservation, logger)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should copy secrets for matching team", func() {
				// Create secrets in the source namespace
				secret1 := &core.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "team-secret-1",
						Namespace: NamespaceHcmAi,
					},
					Data: map[string][]byte{"key1": []byte("value1")},
				}

				// Create a namespace pool with team configuration
				pool := &crd.NamespacePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-development",
						Namespace: NamespaceHcmAi,
					},
					Spec: crd.NamespacePoolSpec{
						Teams: []crd.Team{
							{
								Name: "test-team",
								Secrets: []crd.SecretsData{
									{Name: "team-secret-1"},
								},
							},
						},
					},
				}

				reservation := &crd.NamespaceReservation{
					Spec: crd.NamespaceReservationSpec{
						Team: "test-team",
					},
				}

				Expect(fakeClient.Create(ctx, secret1)).To(Succeed())
				Expect(fakeClient.Create(ctx, pool)).To(Succeed())

				err := CopyReservationSecrets(ctx, fakeClient, "target-ns", reservation, logger)
				Expect(err).ToNot(HaveOccurred())

				// Verify the secret was copied
				copiedSecret := &core.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      "team-secret-1",
					Namespace: "target-ns",
				}, copiedSecret)
				Expect(err).ToNot(HaveOccurred())
				Expect(copiedSecret.Data["key1"]).To(Equal([]byte("value1")))
			})
		})
	})
})
