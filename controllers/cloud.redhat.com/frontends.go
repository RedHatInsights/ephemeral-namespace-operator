package controllers

import (
	"context"
	"fmt"
	"strings"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateFrontendEnv(ctx context.Context, cl client.Client, nsName string, clowdEnv clowder.ClowdEnvironment) error {
	frontendEnv := frontend.FrontendEnvironment{}
	err := cl.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("env-%s", nsName)}, &frontendEnv)

	// if frontendEnv not found create it
	if err != nil && k8serr.IsNotFound(err) {
		// make the hostnames and ingress class on the FrontendEnvironment match that of the ClowdEnvironment
		// we already verified that the hostname was present in the ClowdEnvironment's status via 'verifyClowdEnvReady'
		splitFqdn := strings.Split(clowdEnv.Status.Hostname, ".")
		host := splitFqdn[0]
		var domain string
		if len(splitFqdn) > 1 {
			domain = strings.Join(splitFqdn[1:], ".")
		}

		var ssoUrl string
		if domain == "" {
			ssoUrl = fmt.Sprintf("https://%s-auth/auth/", host)
		} else {
			ssoUrl = fmt.Sprintf("https://%s-auth.%s/auth/", host, domain)
		}

		frontendEnv = frontend.FrontendEnvironment{
			Spec: frontend.FrontendEnvironmentSpec{
				Hostname:     clowdEnv.Status.Hostname,
				SSO:          ssoUrl,
				IngressClass: clowdEnv.Spec.Providers.Web.IngressClass,
			},
		}

		ns, err := GetNamespace(ctx, cl, nsName)
		if err != nil {
			return err
		}

		frontendEnv.SetName(fmt.Sprintf("env-%s", ns.Name))
		frontendEnv.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: ns.APIVersion,
				Kind:       ns.Kind,
				Name:       ns.Name,
				UID:        ns.UID,
			},
		})

		err = cl.Create(ctx, &frontendEnv)
		return err
	}

	return err
}
