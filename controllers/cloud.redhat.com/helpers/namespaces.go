package helpers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
	projectv1 "github.com/openshift/api/project/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateNamespace creates a new ephemeral namespace or project with a random suffix
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

// UpdateNamespaceResources configures a namespace with pool-defined resources including ClowdEnvironment, LimitRange, ResourceQuotas, and secrets
func UpdateNamespaceResources(ctx context.Context, cl client.Client, pool *crd.NamespacePool, nsName string) (core.Namespace, error) {
	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return ns, fmt.Errorf("could not retrieve namespace [%s]: %w", nsName, err)
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

// GetNamespace retrieves a namespace by name with retry logic
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

// GetReadyNamespaces returns all ready namespaces for a given pool
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

// CheckReadyStatus appends a namespace to the ready list if it has the ready status annotation
func CheckReadyStatus(pool string, namespace core.Namespace, ready []core.Namespace) []core.Namespace {
	if val := namespace.Labels[LabelPool]; val == pool {
		if val, ok := namespace.Annotations[AnnotationEnvStatus]; ok && val == EnvStatusReady {
			ready = append(ready, namespace)
		}
	}

	return ready
}

// UpdateAnnotations updates annotations on a namespace with retry logic
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

func logSecretCopyOperation(log logr.Logger, message string, targetNamespace string, secretNames []string) {
	log.Info(message,
		"operation", "copy_summary",
		"source_namespace", NamespaceEphemeralBase,
		"target_namespace", targetNamespace,
		"secret_names", secretNames,
	)
}

// CopySecrets copies Quay pull secrets and other required secrets from ephemeral-base namespace to the target namespace
func CopySecrets(ctx context.Context, cl client.Client, namespaceName string) error {
	// Extract logger from context or create a default one
	log := ctrl.Log.WithName("secret-copy")
	if logPtr := ctx.Value(ContextKey("log")); logPtr != nil {
		if l, ok := logPtr.(*logr.Logger); ok {
			log = *l
		}
	}

	secrets := core.SecretList{}
	if err := cl.List(ctx, &secrets, client.InNamespace(NamespaceEphemeralBase)); err != nil {
		log.Error(err, "SECRET_COPY_OPS: Failed to list secrets from source namespace",
			"operation", "list_secrets",
			"source_namespace", NamespaceEphemeralBase,
			"target_namespace", namespaceName,
		)
		return fmt.Errorf("could not list secrets in [%s]: %w", NamespaceEphemeralBase, err)
	}

	totalSecrets := len(secrets.Items)
	var secretsSkippedMissingAnnotation []string
	var secretsSkippedAnnotationMismatch []string
	var secretsSkippedBonfireIgnore []string
	var secretsSuccessful []string
	var secretsFailed []string

	log.Info("SECRET_COPY_OPS: Starting secret copy operation",
		"operation", "copy_secrets",
		"source_namespace", NamespaceEphemeralBase,
		"target_namespace", namespaceName,
		"total_secrets_found", totalSecrets,
	)

	for _, secret := range secrets.Items {
		// Filter which secrets should be copied
		// All secrets with the "qontract" annotations are defined in app-interface
		if val, ok := secret.Annotations[QontractIntegrationAnnotation]; !ok {
			secretsSkippedMissingAnnotation = append(secretsSkippedMissingAnnotation, secret.Name)
			continue
		} else if val != OpenShiftVaultSecretsProvider && val != OpenShiftRhcsCertsProvider {
			secretsSkippedAnnotationMismatch = append(secretsSkippedAnnotationMismatch, secret.Name)
			continue
		}

		if val, ok := secret.Annotations[BonfireIgnoreAnnotation]; ok {
			if val == "true" {
				secretsSkippedBonfireIgnore = append(secretsSkippedBonfireIgnore, secret.Name)
				continue
			}
		}

		newNamespaceSecret, err := utils.CopySecret(ctx, cl,
			types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace},
			types.NamespacedName{Name: secret.Name, Namespace: namespaceName})
		if err != nil {
			secretsFailed = append(secretsFailed, secret.Name)
			log.Error(err, "SECRET_COPY_OPS: Failed to copy secret",
				"operation", "copy_secret",
				"source_namespace", secret.Namespace,
				"target_namespace", namespaceName,
				"secret_names", []string{secret.Name},
			)
			continue
		}

		if err := cl.Create(ctx, newNamespaceSecret); err != nil {
			secretsFailed = append(secretsFailed, secret.Name)
			log.Error(err, "SECRET_COPY_OPS: Failed to create secret in target namespace",
				"operation", "create_secret",
				"source_namespace", secret.Namespace,
				"target_namespace", namespaceName,
				"secret_names", []string{secret.Name},
			)
			continue
		}

		secretsSuccessful = append(secretsSuccessful, secret.Name)
	}

	totalAttempted := len(secretsSuccessful) + len(secretsFailed)

	if len(secretsSuccessful) > 0 {
		logSecretCopyOperation(log,
			fmt.Sprintf("SECRET_COPY_OPS: %d (of %d) secrets were successfully copied", len(secretsSuccessful), totalAttempted),
			namespaceName,
			secretsSuccessful,
		)
	}

	if len(secretsFailed) > 0 {
		logSecretCopyOperation(log,
			fmt.Sprintf("SECRET_COPY_OPS: %d secrets failed", len(secretsFailed)),
			namespaceName,
			secretsFailed,
		)
	}

	if len(secretsSkippedMissingAnnotation) > 0 {
		logSecretCopyOperation(log,
			fmt.Sprintf("SECRET_COPY_OPS: %d secrets w/ missing qontract annotation", len(secretsSkippedMissingAnnotation)),
			namespaceName,
			secretsSkippedMissingAnnotation,
		)
	}

	if len(secretsSkippedAnnotationMismatch) > 0 {
		logSecretCopyOperation(log,
			fmt.Sprintf("SECRET_COPY_OPS: %d secrets w/ qontract annotation value mismatch", len(secretsSkippedAnnotationMismatch)),
			namespaceName,
			secretsSkippedAnnotationMismatch,
		)
	}

	if len(secretsSkippedBonfireIgnore) > 0 {
		logSecretCopyOperation(log,
			fmt.Sprintf("SECRET_COPY_OPS: %d secrets w/ bonfire ignore annotation set", len(secretsSkippedBonfireIgnore)),
			namespaceName,
			secretsSkippedBonfireIgnore,
		)
	}

	if len(secretsFailed) > 0 {
		return fmt.Errorf("failed to copy %d secrets to namespace [%s]", len(secretsFailed), namespaceName)
	}

	return nil
}

// DeleteNamespace marks a namespace for deletion and removes it from the cluster
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
