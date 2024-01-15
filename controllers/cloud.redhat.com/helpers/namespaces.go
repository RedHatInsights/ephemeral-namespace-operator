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
			return ns.Name, fmt.Errorf("could not create namespace [%s]: %w", ns.Name, err)
		}
	} else {
		project := projectv1.ProjectRequest{}
		project.Name = ns.Name
		if err := cl.Create(ctx, &project); err != nil {
			return ns.Name, fmt.Errorf("could not create project [%s]: %w", project.Name, err)
		}
	}

	return ns.Name, nil
}

func UpdateNamespaceResources(ctx context.Context, cl client.Client, pool *crd.NamespacePool, nsName string) (core.Namespace, error) {
	// WORKAROUND: Can't set annotations and ownerref on project request during create
	// Performing annotation and ownerref change in one transaction
	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return ns, fmt.Errorf("could not retrieve newly created namespace [%s]: %w", nsName, err)
	}

	utils.UpdateAnnotations(&ns, CreateInitialAnnotations())
	utils.UpdateLabels(&ns, CreateInitialLabels(pool.Name))
	ns.SetOwnerReferences([]metav1.OwnerReference{pool.MakeOwnerReference()})

	if err := cl.Update(ctx, &ns); err != nil {
		return ns, fmt.Errorf("could not update Project [%s]: %w", nsName, err)
	}

	// Create ClowdEnvironment
	if err := CreateClowdEnv(ctx, cl, pool.Spec.ClowdEnvironment, nsName); err != nil {
		return ns, fmt.Errorf("error creating ClowdEnvironment for namespace [%s]: %w", nsName, err)
	}

	// Create LimitRange
	limitRange := pool.Spec.LimitRange
	limitRange.SetNamespace(nsName)

	if err := cl.Create(ctx, &limitRange); err != nil {
		return ns, fmt.Errorf("error creating LimitRange for namespace [%s]: %w", nsName, err)
	}

	// Create ResourceQuotas
	resourceQuotas := pool.Spec.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		innerQuota := quota
		innerQuota.SetNamespace(nsName)
		if err := cl.Create(ctx, &innerQuota); err != nil {
			return ns, fmt.Errorf("error creating ResourceQuota for namespace [%s]: %w", nsName, err)
		}
	}

	// Copy secrets
	if err := CopySecrets(ctx, cl, nsName); err != nil {
		return ns, fmt.Errorf("error copying secrets from ephemeral-base namespace to namespace [%s]: %w", nsName, err)
	}

	return ns, nil
}

func GetNamespace(ctx context.Context, cl client.Client, namespaceName string) (core.Namespace, error) {
	namespace := core.Namespace{}

	// Use retry in case object retrieval is attempted before creation is done
	err := retry.OnError(
		wait.Backoff(retry.DefaultBackoff),
		func(error) bool { return true },
		func() error {
			err := cl.Get(ctx, types.NamespacedName{Name: namespaceName}, &namespace)
			return err
		},
	)
	if err != nil {
		return core.Namespace{}, err
	}

	return namespace, nil
}

func GetReadyNamespaces(ctx context.Context, cl client.Client, poolName string) ([]core.Namespace, error) {
	namespaceList := core.NamespaceList{}

	var LabelPoolType = CustomLabel{Label: LabelPool, Value: poolName}
	validatedSelector, _ := labels.ValidatedSelectorFromSet(LabelPoolType.ToMap())

	namespaceListOptions := &client.ListOptions{LabelSelector: validatedSelector}

	if err := cl.List(ctx, &namespaceList, namespaceListOptions); err != nil {
		return nil, fmt.Errorf("error listing namespaces for pool [%s]: %w", poolName, err)
	}

	var ready []core.Namespace

	for _, namespace := range namespaceList.Items {
		for _, owner := range namespace.GetOwnerReferences() {
			if owner.Kind == KindNamespacePool {
				ready = CheckReadyStatus(poolName, namespace, ready)
			}
		}
	}

	return ready, nil
}

func CheckReadyStatus(pool string, namespace core.Namespace, ready []core.Namespace) []core.Namespace {
	if val := namespace.ObjectMeta.Labels[LabelPool]; val == pool {
		if val, ok := namespace.ObjectMeta.Annotations[AnnotationEnvStatus]; ok && val == EnvStatusReady {
			ready = append(ready, namespace)
		}
	}

	return ready
}

func UpdateAnnotations(ctx context.Context, cl client.Client, namespaceName string, annotations map[string]string) error {
	namespace, err := GetNamespace(ctx, cl, namespaceName)
	if err != nil {
		return fmt.Errorf("error updating annotations for namespace [%s]: %w", namespaceName, err)
	}

	utils.UpdateAnnotations(&namespace, annotations)

	err = retry.RetryOnConflict(
		retry.DefaultBackoff,
		func() error {
			if err = cl.Update(ctx, &namespace); err != nil {
				return fmt.Errorf("there was an issue updating annotations for namespace [%s]: %w", namespaceName, err)
			}

			return nil
		},
	)

	return nil
}

func CopySecrets(ctx context.Context, cl client.Client, namespaceName string) error {
	secrets := core.SecretList{}
	if err := cl.List(ctx, &secrets, client.InNamespace(NamespaceEphemeralBase)); err != nil {
		return fmt.Errorf("could not list secrets in [%s]: %w", NamespaceEphemeralBase, err)
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
	return nil
}

func DeleteNamespace(ctx context.Context, cl client.Client, namespaceName string) error {
	if err := UpdateAnnotations(ctx, cl, namespaceName, AnnotationEnvDeleting.ToMap()); err != nil {
		return fmt.Errorf("error updating annotations for [%s]: %w", namespaceName, err)
	}

	namespace, err := GetNamespace(ctx, cl, namespaceName)
	if err != nil {
		return fmt.Errorf("could not retrieve namespace [%s] to be deleted: %w", namespaceName, err)
	}

	if err := cl.Delete(ctx, &namespace); err != nil {
		return fmt.Errorf("could not delete namespace [%s]: %w", namespaceName, err)
	}

	return nil
}
