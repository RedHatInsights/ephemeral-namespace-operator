package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SetupNamespace(ctx context.Context, cl client.Client, cfg OperatorConfig, log logr.Logger) (string, error) {
	// Create new namespace
	ns, err := CreateNamespace(ctx, cl, cfg.PoolConfig.Local)
	if err != nil {
		log.Error(err, "Error creating new namespace")
		return "", err
	}
	log.Info("Setting up new ephemeral namespace", "ns-name", ns)

	// Create ClowdEnvironment
	log.Info("Creating new ClowdEnvironment", "ns-name", ns)
	err = CreateClowdEnv(ctx, cl, cfg.ClowdEnvSpec, ns)
	if err != nil {
		log.Error(err, "Error creating ClowdEnvironment", "ns-name", ns)
		return "", err
	}

	// Set initial annotations on ns
	log.Info("Setting initial annotations on ns", "ns-name", ns)
	initialAnnotations := map[string]string{
		"pool-status": "creating",
		"operator-ns": "true",
	}
	if err := UpdateAnnotations(ctx, cl, initialAnnotations, ns); err != nil {
		log.Error(err, "Could not update namespace annotations", "ns-name", ns)
		return "", err
	}

	// Create LimitRange
	limitRange := cfg.LimitRange
	limitRange.SetNamespace(ns)
	if err := cl.Create(ctx, &limitRange); err != nil {
		log.Error(err, "Cannot create LimitRange in Namespace", "ns-name", ns)
		return "", err
	}

	// Create ResourceQuotas
	resourceQuotas := cfg.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		quota.SetNamespace(ns)
		if err := cl.Create(ctx, &quota); err != nil {
			log.Error(err, "Cannot create ResourceQuota in Namespace", "ns-name", ns)
			return "", err
		}
	}

	// Copy secrets
	if err := CopySecrets(ctx, cl, ns); err != nil {
		log.Error(err, "Could not copy secrets into namespace", "ns-name", ns)
		return "", err
	}

	// Wait for clowdenv to go ready before proceeding
	// TODO: Add timeout
	// TODO: Instead of waiting - watch clowdenvs for updates in controller
	log.Info("Verifying that the ClowdEnv is ready for namespace", "ns-name", ns)
	clowdEnv := WaitForClowdEnv(ctx, cl, log, ns)

	// Create FrontendEnvironment if ClowdEnvironment's web provider is set to 'local' mode
	if clowdEnv.Spec.Providers.Web.Mode == "local" {
		if err := CreateFrontendEnv(ctx, cl, ns, *clowdEnv); err != nil {
			return "", err
		}
	}

	log.Info("Namespace setup completed successfully", "ns-name", ns)

	return ns, nil
}
