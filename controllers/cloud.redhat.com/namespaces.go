package controllers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	projectv1 "github.com/openshift/api/project/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var initialAnnotations = map[string]string{
	"status":      "creating",
	"operator-ns": "true",
}

func CreateNamespace(ctx context.Context, cl client.Client, pool *crd.Pool, local bool) (*core.Namespace, error) {
	// Create project or namespace depending on environment
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))

	if local {
		if err := cl.Create(ctx, &ns); err != nil {
			return nil, err
		}
	} else {
		project := projectv1.ProjectRequest{}
		project.Name = ns.Name
		if err := cl.Create(ctx, &project); err != nil {
			return nil, err
		}
	}

	if err := UpdateAnnotations(ctx, cl, initialAnnotations, ns.Name); err != nil {
		return &ns, errors.New(fmt.Sprintf("Error setting initial annotations: %s", err))
	}

	if err := UpdateOwnerRef(ctx, cl, ns.Name, []metav1.OwnerReference{pool.MakeOwnerReference()}); err != nil {
		return &ns, errors.New(fmt.Sprintf("Error updating ns owner reference to pool: %s", err))
	}

	return &ns, nil
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

func GetNamespacesByStatus(ctx context.Context, cl client.Client, status string) ([]core.Namespace, error) {
	nsList := core.NamespaceList{}
	if err := cl.List(ctx, &nsList); err != nil {
		return nil, err
	}

	var ready []core.Namespace

	for _, ns := range nsList.Items {
		matched, _ := regexp.MatchString(`ephemeral-\w{6}$`, ns.Name)
		if matched {
			if val, ok := ns.ObjectMeta.Annotations["status"]; ok && val == status {
				ready = append(ready, ns)
			}
		}
	}

	return ready, nil
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

func UpdateOwnerRef(ctx context.Context, cl client.Client, nsName string, owner []metav1.OwnerReference) error {
	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
		return err
	}

	ns.SetOwnerReferences(owner)

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
