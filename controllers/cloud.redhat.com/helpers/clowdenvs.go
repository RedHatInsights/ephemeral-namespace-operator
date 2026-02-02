// Package helpers provides utility functions for managing namespace resources, ClowdEnvironments, and FrontendEnvironments
package helpers

import (
	"context"
	"fmt"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateClowdEnv creates a ClowdEnvironment resource in the specified namespace
func CreateClowdEnv(ctx context.Context, cl client.Client, spec clowder.ClowdEnvironmentSpec, namespaceName string) error {
	env := clowder.ClowdEnvironment{
		Spec: spec,
	}
	env.SetName(fmt.Sprintf("env-%s", namespaceName))
	env.Spec.TargetNamespace = namespaceName

	ns, err := GetNamespace(ctx, cl, namespaceName)
	if err != nil {
		return fmt.Errorf("could not retrieve namespace [%s] for setting the owner reference on Clowdenvironment [%s]: %w", namespaceName, env.Name, err)
	}

	env.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: ns.APIVersion,
			Kind:       ns.Kind,
			Name:       ns.Name,
			UID:        ns.UID,
		},
	})

	if err := cl.Create(ctx, &env); err != nil {
		return fmt.Errorf("could not create clowdenvironment for namespace [%s]: %w", namespaceName, err)
	}

	return nil
}

// GetClowdEnv retrieves a ClowdEnvironment resource and checks if it's ready
func GetClowdEnv(ctx context.Context, cl client.Client, namespaceName string) (bool, *clowder.ClowdEnvironment, error) {
	env := clowder.ClowdEnvironment{}
	nn := types.NamespacedName{
		Name:      fmt.Sprintf("env-%s", namespaceName),
		Namespace: namespaceName,
	}

	err := cl.Get(ctx, nn, &env)
	if err != nil {
		return false, nil, fmt.Errorf("could not retrieve clowdenvironment [%s]: %w", env.Name, err)
	}

	ready := VerifyClowdEnvReady(env)
	if !ready {
		return ready, &env, nil
	}

	return ready, &env, nil
}

// VerifyClowdEnvReady checks if a ClowdEnvironment has completed reconciliation and all deployments are ready
func VerifyClowdEnvReady(env clowder.ClowdEnvironment) bool {
	// check that hostname is populated if ClowdEnvironment is operating in 'local' web mode
	if env.Spec.Providers.Web.Mode == "local" && env.Status.Hostname == "" {
		return false
	}

	conditions := env.Status.Conditions

	reconciliationSuccessful := false
	deploymentsReady := false

	for i := range conditions {
		if conditions[i].Type == "ReconciliationSuccessful" && conditions[i].Status == "True" {
			reconciliationSuccessful = true
		}
		if conditions[i].Type == "DeploymentsReady" && conditions[i].Status == "True" {
			deploymentsReady = true
		}
	}

	return (reconciliationSuccessful && deploymentsReady)
}
