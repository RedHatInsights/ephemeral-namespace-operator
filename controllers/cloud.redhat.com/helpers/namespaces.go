package helpers

import (
	"context"
	"errors"
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

	annotations := CustomAnnotation{}
	annotations.SetInitialAnnotations(&ns)

	labels := CustomLabel{}
	labels.SetInitialLabels(&ns, pool.Name)

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

	ns.SetOwnerReferences([]metav1.OwnerReference{pool.MakeOwnerReference()})

	if err := cl.Update(ctx, &ns); err != nil {
		return ns.Name, err
	}

	// Create ClowdEnvironment
	if err := CreateClowdEnv(ctx, cl, pool.Spec.ClowdEnvironment, ns.Name); err != nil {
		return "", errors.New("Error creating ClowdEnvironment: " + err.Error())
	}

	// Create LimitRange
	limitRange := pool.Spec.LimitRange
	limitRange.SetNamespace(ns.Name)

	if err := cl.Create(ctx, &limitRange); err != nil {
		return "", errors.New("Error creating LimitRange: " + err.Error())
	}

	// Create ResourceQuotas
	resourceQuotas := pool.Spec.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		quota.SetNamespace(ns.Name)
		if err := cl.Create(ctx, &quota); err != nil {
			return "", errors.New("Error creating ResourceQuota: " + err.Error())
		}
	}

	// Copy secrets
	if err := CopySecrets(ctx, cl, ns.Name); err != nil {
		return "", errors.New("Error copying secrets: " + err.Error())
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

func GetReadyNamespaces(ctx context.Context, cl client.Client, pool string) ([]core.Namespace, error) {
	nsList := core.NamespaceList{}

	validatedSelector, _ := labels.ValidatedSelectorFromSet(
		map[string]string{LABEL_POOL: pool})

	nsListOptions := &client.ListOptions{LabelSelector: validatedSelector}

	if err := cl.List(ctx, &nsList, nsListOptions); err != nil {
		return nil, err
	}

	var ready []core.Namespace

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.Kind == KIND_NAMESPACEPOOL {
				ready = CheckReadyStatus(pool, ns, ready)
			}
		}
	}

	return ready, nil
}

func CheckReadyStatus(pool string, ns core.Namespace, ready []core.Namespace) []core.Namespace {
	if val := ns.ObjectMeta.Labels[LABEL_POOL]; val == pool {
		if val, ok := ns.ObjectMeta.Annotations[ANNOTATION_ENV_STATUS]; ok && val == ENV_STATUS_READY {
			ready = append(ready, ns)
		}
	}

	return ready
}

func UpdateAnnotations(ctx context.Context, cl client.Client, nsName string, annotations map[string]string) error {
	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return err
	}

	utils.UpdateAnnotations(&ns, annotations)

	if err := cl.Update(ctx, &ns); err != nil {
		return err
	}

	return nil
}

func CopySecrets(ctx context.Context, cl client.Client, nsName string) error {
	secrets := core.SecretList{}
	if err := cl.List(ctx, &secrets, client.InNamespace(NAMESPACE_EPHEMERAL_BASE)); err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		// Filter which secrets should be copied
		// All secrets with the "qontract" annotations are defined in app-interface
		if val, ok := secret.Annotations[QONTRACT_INTEGRATION_SECRET]; !ok {
			continue
		} else {
			if val != OPENSHIFT_VAULT_SECRETS_SECRET {
				continue
			}
		}

		if val, ok := secret.Annotations[BONFIRE_IGNORE_SECRET]; ok {
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
	UpdateAnnotations(ctx, cl, nsName, AnnotationEnvDeleting.ToMap())

	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return err
	}

	if err := cl.Delete(ctx, &ns); err != nil {
		return err
	}

	return nil
}
