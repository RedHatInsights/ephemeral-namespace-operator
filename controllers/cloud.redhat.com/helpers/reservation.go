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

	// Find the team configuration
	team := findTeamByName(allTeams, teamName)
	if team == nil {
		log.Info(fmt.Sprintf("Team [%s] not found in namespace pool configuration", teamName))
		return nil
	}

	// Copy secrets for the team
	return copyTeamSecrets(ctx, cl, secrets.Items, team.Secrets, namespaceName, log)
}

// findTeamByName searches for a team by name in the teams slice
func findTeamByName(teams []crd.Teams, teamName string) *crd.Teams {
	for _, team := range teams {
		if team.Name == teamName {
			return &team
		}
	}
	return nil
}

// copyTeamSecrets copies all secrets configured for a team to the target namespace
func copyTeamSecrets(ctx context.Context, cl client.Client, availableSecrets []core.Secret, teamSecrets []crd.SecretsData, targetNamespace string, log logr.Logger) error {
	for _, teamSecret := range teamSecrets {
		secret := findSecretByName(availableSecrets, teamSecret.Name)
		if secret == nil {
			log.Info(fmt.Sprintf("Secret [%s] not found in source namespace", teamSecret.Name))
			continue
		}

		if err := copySecretToNamespace(ctx, cl, secret, teamSecret, targetNamespace); err != nil {
			return fmt.Errorf("failed to copy secret [%s] to namespace [%s]: %w", teamSecret.Name, targetNamespace, err)
		}
	}
	return nil
}

// findSecretByName searches for a secret by name in the secrets slice
func findSecretByName(secrets []core.Secret, secretName string) *core.Secret {
	for _, secret := range secrets {
		if secret.Name == secretName {
			return &secret
		}
	}
	return nil
}

// copySecretToNamespace copies a single secret to the target namespace with proper naming
func copySecretToNamespace(ctx context.Context, cl client.Client, secret *core.Secret, teamSecret crd.SecretsData, targetNamespace string) error {
	sourceNamespaceName := types.NamespacedName{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}

	// Determine the destination secret name
	destinationSecretName := teamSecret.Name
	if teamSecret.DestName != "" {
		destinationSecretName = teamSecret.DestName
	}

	destinationNamespaceName := types.NamespacedName{
		Name:      destinationSecretName,
		Namespace: targetNamespace,
	}

	newNamespaceSecret, err := utils.CopySecret(ctx, cl, sourceNamespaceName, destinationNamespaceName)
	if err != nil {
		return fmt.Errorf("could not copy secret: %w", err)
	}

	if err := cl.Create(ctx, newNamespaceSecret); err != nil {
		return fmt.Errorf("could not create secret: %w", err)
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
