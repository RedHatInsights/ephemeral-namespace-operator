package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	projectv1 "github.com/openshift/api/project/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var initialAnnotations = map[string]string{
	"status":      "creating", // TODO: Remove this annotation after Bonfire is updated
	"env-status":  "creating",
	"operator-ns": "true",
}

var initialLabels = map[string]string{
	"operator-ns": "true",
}

func CreateNamespace(ctx context.Context, cl client.Client, pool *crd.NamespacePool) (string, error) {
	// Create project or namespace depending on environment
	ns := core.Namespace{}

	labels := map[string]string{}
	for k, v := range initialLabels {
		labels[k] = v
	}

	labels["pool"] = pool.Name

	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))

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

	if len(ns.Annotations) == 0 {
		ns.SetAnnotations(initialAnnotations)
	} else {
		for k, v := range initialAnnotations {
			ns.Annotations[k] = v
		}
	}

	if len(ns.Labels) == 0 {
		ns.SetLabels(labels)
	} else {
		for k, v := range labels {
			ns.Labels[k] = v
		}
	}

	ns.SetOwnerReferences([]metav1.OwnerReference{pool.MakeOwnerReference()})

	if err := cl.Update(ctx, &ns); err != nil {
		return ns.Name, err
	}

	return ns.Name, nil
}

func SetupNamespace(ctx context.Context, cl client.Client, pool crd.NamespacePool, ns string) error {
	labels := map[string]string{}

	for k, v := range initialLabels {
		labels[k] = v
	}

	labels["pool"] = pool.Name

	// Create ClowdEnvironment
	if err := CreateClowdEnv(ctx, cl, pool.Spec.ClowdEnvironment, ns); err != nil {
		return errors.New("Error creating ClowdEnvironment: " + err.Error())
	}

	// Create LimitRange
	limitRange := pool.Spec.LimitRange
	limitRange.SetNamespace(ns)

	if err := cl.Create(ctx, &limitRange); err != nil {
		return errors.New("Error creating LimitRange: " + err.Error())
	}

	// Create ResourceQuotas
	resourceQuotas := pool.Spec.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		quota.SetNamespace(ns)
		if err := cl.Create(ctx, &quota); err != nil {
			return errors.New("Error creating ResourceQuota: " + err.Error())
		}
	}

	// Copy secrets
	if err := CopySecrets(ctx, cl, ns); err != nil {
		return errors.New("Error copying secrets: " + err.Error())
	}

	return nil
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
		map[string]string{"operator-ns": "true", "pool": pool})

	nsListOptions := &client.ListOptions{LabelSelector: validatedSelector}

	if err := cl.List(ctx, &nsList, nsListOptions); err != nil {
		return nil, err
	}

	var ready []core.Namespace

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.Kind == "NamespacePool" {
				ready = CheckReadyStatus(pool, ns, ready)
			}
		}
	}

	return ready, nil
}

func CheckReadyStatus(pool string, ns core.Namespace, ready []core.Namespace) []core.Namespace {
	if val := ns.ObjectMeta.Labels["pool"]; val == pool {
		if val, ok := ns.ObjectMeta.Annotations["env-status"]; ok && val == "ready" {
			ready = append(ready, ns)
		}
	}

	return ready
}

func UpdateAnnotations(ctx context.Context, cl client.Client, annotations map[string]string, nsName string) error {
	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return err
	}

	if len(ns.Annotations) == 0 {
		ns.SetAnnotations(annotations)
	} else {
		for k, v := range annotations {
			ns.Annotations[k] = v
		}
	}

	if err := cl.Update(ctx, &ns); err != nil {
		return err
	}

	return nil
}

func CopySecrets(ctx context.Context, cl client.Client, nsName string) error {
	secrets := core.SecretList{}
	if err := cl.List(ctx, &secrets, client.InNamespace("ephemeral-base")); err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		// Filter which secrets should be copied
		// All secrets with the "qontract" annotations are defined in app-interface
		if val, ok := secret.Annotations["qontract.integration"]; !ok {
			continue
		} else {
			if val != "openshift-vault-secrets" {
				continue
			}
		}

		if val, ok := secret.Annotations["bonfire.ignore"]; ok {
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

		err, newNsSecret := utils.CopySecret(ctx, cl, src, dst)
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
	UpdateAnnotations(ctx, cl, map[string]string{"env-status": "deleting", "status": "deleting"}, nsName)

	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return err
	}

	if err := cl.Delete(ctx, &ns); err != nil {
		return err
	}

	return nil
}

func GetPrometheusOperatorName(nsName string) string {
	return fmt.Sprintf("prometheus.%s", nsName)
}

func DeletePrometheusOperator(ctx context.Context, cl client.Client, nsName string) error {
	prometheusOperator := unstructured.Unstructured{}

	err := cl.Get(ctx, types.NamespacedName{Name: GetPrometheusOperatorName(nsName)}, &prometheusOperator)
	if err != nil {
		return err
	}

	gvk := schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1",
		Kind:    "Operator",
	}

	prometheusOperator.SetGroupVersionKind(gvk)
	prometheusOperator.SetName(GetPrometheusOperatorName(nsName))

	err = cl.Delete(ctx, &prometheusOperator)
	if err != nil {
		return fmt.Errorf("error deleting prometheus operator %s: %v", GetPrometheusOperatorName(nsName), err)
	}

	return nil
}

func DeleteSubscriptionPrometheusOperator(ctx context.Context, cl client.Client, nsName string) error {
	subscriptionsPrometheusOperator := unstructured.Unstructured{}

	err := cl.Get(ctx, types.NamespacedName{Name: "prometheus", Namespace: nsName}, &subscriptionsPrometheusOperator)
	if err != nil {
		return fmt.Errorf("error retrieving prometheus operator subscription in namespace %s: %v", nsName, err)
	}

	gvk := schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	}

	subscriptionsPrometheusOperator.SetGroupVersionKind(gvk)
	subscriptionsPrometheusOperator.SetName(GetPrometheusOperatorName(nsName))

	for {
		err = cl.Delete(ctx, &subscriptionsPrometheusOperator)
		if err != nil {
			return fmt.Errorf("error deleting prometheus operator subscription in namespace %s: %v", nsName, err)
		}

		err := cl.Get(ctx, types.NamespacedName{Name: "prometheus", Namespace: nsName}, &subscriptionsPrometheusOperator)
		if err != nil {
			fmt.Printf("prometheus operator subscription in namespace %s was successfully deleted", nsName)
			break
		}
	}

	return nil
}
