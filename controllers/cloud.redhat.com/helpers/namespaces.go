package helpers

import (
	"context"
	"fmt"
	"strings"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	projectv1 "github.com/openshift/api/project/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateNamespace(ctx context.Context, cl client.Client, pool *crd.NamespacePool) (string, error) {
	ns := core.Namespace{}

	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(utils.RandString(6)))

	if pool.Spec.Local {
		if err := cl.Create(ctx, &ns); err != nil {
			return "", err
		}
	} else {
		project := projectv1.ProjectRequest{}
		project.Name = ns.Name
		if err := cl.Create(ctx, &project); err != nil {
			return "", err
		}
	}

	// WORKAROUND: Can't set annotations and ownerref on project request during create
	// Performing annotation and ownerref change in one transaction
	ns, err := GetNamespace(ctx, cl, ns.Name)
	if err != nil {
		return ns.Name, err
	}

	utils.UpdateAnnotations(&ns, CreateInitialAnnotations())
	utils.UpdateLabels(&ns, CreateInitialLabels(pool.Name))
	ns.SetOwnerReferences([]metav1.OwnerReference{pool.MakeOwnerReference()})

	if err := cl.Update(ctx, &ns); err != nil {
		return ns.Name, err
	}

	// Create ClowdEnvironment
	if err := CreateClowdEnv(ctx, cl, pool.Spec.ClowdEnvironment, ns.Name); err != nil {
		return "", fmt.Errorf("error creating ClowdEnvironment: %s", err.Error())
	}

	// Create LimitRange
	limitRange := pool.Spec.LimitRange
	limitRange.SetNamespace(ns.Name)

	if err := cl.Create(ctx, &limitRange); err != nil {
		return "", fmt.Errorf("error creating LimitRange: %s", err.Error())
	}

	// Create ResourceQuotas
	resourceQuotas := pool.Spec.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		innerQuota := quota
		innerQuota.SetNamespace(ns.Name)
		if err := cl.Create(ctx, &innerQuota); err != nil {
			return "", fmt.Errorf("error creating ResourceQuota: %s", err.Error())
		}
	}

	// Copy secrets
	if err := CopySecrets(ctx, cl, ns.Name); err != nil {
		return "", fmt.Errorf("error copying secrets from ephemeral-base namespace: %s", err.Error())
	}

	return ns.Name, nil
}

func GetNamespace(ctx context.Context, cl client.Client, nsName string) (core.Namespace, error) {
	ns := core.Namespace{}

	// Use retry in case object retrieval is attempted before creation is done
	err := retry.OnError(
		wait.Backoff(retry.DefaultBackoff),
		func(error) bool { return true }, // hack - return true if err is notFound
		func() error {
			err := cl.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
			return err
		},
	)
	if err != nil {
		return core.Namespace{}, err
	}

	return ns, nil
}

func GetReadyNamespaces(ctx context.Context, cl client.Client, poolName string) ([]core.Namespace, error) {
	nsList := core.NamespaceList{}

	var LabelPoolType = CustomLabel{Label: LabelPool, Value: poolName}
	validatedSelector, _ := labels.ValidatedSelectorFromSet(LabelPoolType.ToMap())

	nsListOptions := &client.ListOptions{LabelSelector: validatedSelector}

	if err := cl.List(ctx, &nsList, nsListOptions); err != nil {
		return nil, err
	}

	var ready []core.Namespace

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.Kind == KindNamespacePool {
				ready = CheckReadyStatus(poolName, ns, ready)
			}
		}
	}

	return ready, nil
}

func CheckReadyStatus(pool string, ns core.Namespace, ready []core.Namespace) []core.Namespace {
	if val := ns.ObjectMeta.Labels[LabelPool]; val == pool {
		if val, ok := ns.ObjectMeta.Annotations[AnnotationEnvStatus]; ok && val == EnvStatusReady {
			ready = append(ready, ns)
		}
	}

	return ready
}

func UpdateAnnotations(ctx context.Context, cl client.Client, namespaceName string, annotations map[string]string) error {
	namespace, err := GetNamespace(ctx, cl, namespaceName)
	if err != nil {
		return err
	}

	utils.UpdateAnnotations(&namespace, annotations)

	if err := cl.Get(ctx, types.NamespacedName{Name: namespaceName}, &namespace); err != nil {
		return fmt.Errorf("there was issue retrieving namespace [%s] with newly added annotations", namespaceName)
	}

	if err := cl.Update(ctx, &namespace); err != nil {
		return fmt.Errorf("there was issue updating annotations for namespace [%s]", namespaceName)
	}

	return nil
}

func CopySecrets(ctx context.Context, cl client.Client, nsName string) error {
	secrets := core.SecretList{}
	if err := cl.List(ctx, &secrets, client.InNamespace(NamespaceEphemeralBase)); err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		// Filter which secrets should be copied
		// All secrets with the "qontract" annotations are defined in app-interface
		if val, ok := secret.Annotations[QontractIntegrationSecret]; !ok {
			continue
		} else if val != OpenShiftVaultSecretsSecret {
			continue
		}

		if val, ok := secret.Annotations[BonfireGinoreSecret]; ok {
			if val == "true" {
				continue
			}
		}

		src := types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}

		dst := types.NamespacedName{
			Name:      secret.Name,
			Namespace: nsName,
		}

		newNsSecret, err := utils.CopySecret(ctx, cl, src, dst)
		if err != nil {
			return err
		}

		if err := cl.Create(ctx, newNsSecret); err != nil {
			return err
		}

	}
	return nil
}

func DeleteNamespace(ctx context.Context, cl client.Client, nsName string) error {
	if err := UpdateAnnotations(ctx, cl, nsName, AnnotationEnvDeleting.ToMap()); err != nil {
		return fmt.Errorf("error updating annotations: %w", err)
	}

	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return err
	}

	if err := cl.Delete(ctx, &ns); err != nil {
		return err
	}

	return nil
}
