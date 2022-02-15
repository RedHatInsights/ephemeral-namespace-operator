package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errorAnnotations = map[string]string{
	"status": "error",
}
var readyAnnotations = map[string]string{
	"status": "ready",
}

// Provide a newly created namespace to be configured
func SetupNamespace(ctx context.Context, cl client.Client, cfg OperatorConfig, log logr.Logger, ns string) {
	// Create ClowdEnvironment
	log.Info("Creating new ClowdEnvironment", "ns-name", ns)
	if err := CreateClowdEnv(ctx, cl, cfg.ClowdEnvSpec, ns); err != nil {
		log.Error(err, "Error creating ClowdEnvironment", "ns-name", ns)
		UpdateAnnotations(ctx, cl, errorAnnotations, ns)
		return
	}

	// Create LimitRange
	limitRange := cfg.LimitRange
	limitRange.SetNamespace(ns)
	if err := cl.Create(ctx, &limitRange); err != nil {
		log.Error(err, "Cannot create LimitRange in Namespace", "ns-name", ns)
		UpdateAnnotations(ctx, cl, errorAnnotations, ns)
		return
	}

	// Create ResourceQuotas
	resourceQuotas := cfg.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		quota.SetNamespace(ns)
		if err := cl.Create(ctx, &quota); err != nil {
			log.Error(err, "Cannot create ResourceQuota in Namespace", "ns-name", ns)
			UpdateAnnotations(ctx, cl, errorAnnotations, ns)
			return
		}
	}

	// Copy secrets
	if err := CopySecrets(ctx, cl, ns); err != nil {
		log.Error(err, "Could not copy secrets into namespace", "ns-name", ns)
		UpdateAnnotations(ctx, cl, errorAnnotations, ns)
		return
	}

	return
}
