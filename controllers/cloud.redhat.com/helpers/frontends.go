package helpers

import (
	"context"
	"fmt"
	"strings"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateFrontendEnv(ctx context.Context, cl client.Client, namespaceName string, clowdEnv clowder.ClowdEnvironment) error {
	frontendEnv := frontend.FrontendEnvironment{}
	err := cl.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("env-%s", namespaceName)}, &frontendEnv)
	// Checks if frontenv environment exists
	if err == nil {
		return nil
	}

	if !k8serr.IsNotFound(err) {
		return fmt.Errorf("there was an error when retrieving the frontend environment [env-%s]: %w", namespaceName, err)
	}

	// if frontendEnv not found create it
	splitFqdn := strings.Split(clowdEnv.Status.Hostname, ".")
	host := splitFqdn[0]
	var domain string
	if len(splitFqdn) > 1 {
		domain = strings.Join(splitFqdn[1:], ".")
	}

	var ssoURL string
	if domain == "" {
		ssoURL = fmt.Sprintf("https://%s-auth/auth/", host)
	} else {
		ssoURL = fmt.Sprintf("https://%s-auth.%s/auth/", host, domain)
	}

	frontendEnv = frontend.FrontendEnvironment{
		Spec: frontend.FrontendEnvironmentSpec{
			Hostname:        clowdEnv.Status.Hostname,
			SSO:             ssoURL,
			IngressClass:    clowdEnv.Spec.Providers.Web.IngressClass,
			GenerateNavJSON: true,
		},
	}

	namespace, err := GetNamespace(ctx, cl, namespaceName)
	if err != nil {
		return err
	}

	frontendEnv.SetName(fmt.Sprintf("env-%s", namespace.Name))
	frontendEnv.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: namespace.APIVersion,
			Kind:       namespace.Kind,
			Name:       namespace.Name,
			UID:        namespace.UID,
		},
	})

	if err := cl.Create(ctx, &frontendEnv); err != nil {
		return fmt.Errorf("error creating frontend environment [%s]: %w", frontendEnv.Name, err)
	}

	// create "shim" services for keycloak, mbop, mocktitlements
	// this is a temporary solution until apps begin to read the hostnames for these from their cdappconfig.json
	if err := createShimServices(ctx, cl, namespace, clowdEnv); err != nil {
		return fmt.Errorf("creation of shim services for keycloak, mbop, and mocktitlements failed for [%s]: %w", clowdEnv.Name, err)
	}

	return nil
}

func createShimServices(ctx context.Context, cl client.Client, ns core.Namespace, clowdEnv clowder.ClowdEnvironment) error {
	serviceNames := []string{"keycloak", "mbop", "mocktitlements"}

	for _, serviceName := range serviceNames {
		origSvc := core.Service{}
		nn := types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", clowdEnv.Name, serviceName),
			Namespace: ns.Name,
		}

		err := cl.Get(ctx, nn, &origSvc)
		if err != nil {
			continue
		}

		// create a new Service with the same configuration spec but a non-prefixed name
		newSvc := core.Service{}
		newSvc.SetName(serviceName)
		newSvc.SetNamespace(ns.Name)
		newSvc.Spec = origSvc.Spec
		// empty the ClusterIPs so a new one is generated
		newSvc.Spec.ClusterIP = ""
		newSvc.Spec.ClusterIPs = []string{}
		newSvc.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: ns.APIVersion,
				Kind:       ns.Kind,
				Name:       ns.Name,
				UID:        ns.UID,
			},
		})

		err = cl.Create(ctx, &newSvc)
		if err != nil {
			return fmt.Errorf("failed to create shim service for [%s]: %w", serviceName, err)
		}
	}

	return nil
}
