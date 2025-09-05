package helpers

import (
	"context"
	"fmt"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This function handles copying AI secrets
func CopyReservationSecrets(ctx context.Context, cl client.Client, namespaceName string, res *crd.NamespaceReservation, log logr.Logger) error {
	secrets := core.SecretList{}

	nsPoolObject := crd.NamespacePool{}

	err := cl.Get(ctx, types.NamespacedName{Name: "ai-development", Namespace: NamespaceHcmAi}, &nsPoolObject)
	if err != nil {
		log.Error(err, fmt.Sprintf("could not retrieve namespacepool [%s]", PoolAiDevelopment))
	}
	log.Info(fmt.Sprintf("[%s]: %v", PoolAiDevelopment, nsPoolObject))

	err = cl.List(ctx, &secrets, client.InNamespace(NamespaceHcmAi))
	if err != nil {
		return fmt.Errorf("could not list secrets in [%s]: %w", NamespaceHcmAi, err)
	}

	teamName := res.Spec.Team
	allTeams := nsPoolObject.Spec.Teams

	for _, team := range allTeams {
		if team.Name == teamName {
			for _, secret := range secrets.Items {

				for _, teamSecret := range team.Secrets {
					if secret.Name == teamSecret {
						sourceNamespaceName := types.NamespacedName{
							Name:      secret.Name,
							Namespace: secret.Namespace,
						}

						destinationNamespace := types.NamespacedName{
							Name:      secret.Name,
							Namespace: namespaceName,
						}

						newNamespaceSecret, err := utils.CopySecret(ctx, cl, sourceNamespaceName, destinationNamespace)
						if err != nil {
							return fmt.Errorf("could not copy secrets into newly created namespace [%s]: %w", namespaceName, err)
						}

						if err := cl.Create(ctx, newNamespaceSecret); err != nil {
							return fmt.Errorf("could not create new secret for namespace [%s]: %w", namespaceName, err)
						}
					}
				}
			}
		}
	}

	return nil
}

// NewReservation creates a mock reservation for testing
func NewReservation(resName string, duration string, requester string, team string, pool string) *crd.NamespaceReservation {
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
			Team:      team,
			Pool:      pool,
		},
	}
}
