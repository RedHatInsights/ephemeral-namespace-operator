/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	projectv1 "github.com/openshift/api/project/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8serr "k8s.io/apimachinery/pkg/api/errors"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
)

// PoolReconciler reconciles a Pool object
type PoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config OperatorConfig
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=cloud.redhat.com,resources=pools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=pools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.redhat.com,resources=pools/finalizers,verbs=update

func (r *PoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pool := crd.Pool{}
	if err := r.Client.Get(ctx, req.NamespacedName, &pool); err != nil {
		if k8serr.IsNotFound(err) {
			r.Log.Info("Existing pool CRD not found. Creating one from config")
			pool.Spec.Size = r.Config.PoolConfig.Size
			pool.Spec.Local = r.Config.PoolConfig.Local
			if err := r.Client.Create(ctx, &pool); err != nil {
				r.Log.Error(err, "Error creating namespace pool CRD")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "Error retrieving namespace pool")
		return ctrl.Result{}, err
	}

	status, err := r.getPoolStatus(ctx, pool)
	if err != nil {
		r.Log.Error(err, "Unable to get status of owned namespaces")
		return ctrl.Result{}, err
	}

	pool.Status.Ready = status["ready"]
	pool.Status.Creating = status["creating"]
	if err := r.Status().Update(ctx, &pool); err != nil {
		r.Log.Error(err, "Cannot update pool status")
		return ctrl.Result{}, err
	}

	if r.underManaged(pool) {
		if err := r.createNS(ctx, pool); err != nil {
			r.Log.Error(err, "Unable to create namespace")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crd.Pool{}).
		Complete(r)
}

func (r *PoolReconciler) getPoolStatus(ctx context.Context, pool crd.Pool) (map[string]int, error) {
	nsList := core.NamespaceList{}
	if err := r.Client.List(ctx, &nsList); err != nil {
		r.Log.Error(err, "Unable to retrieve list of existing ready namespaces")
		return nil, err
	}

	var readyNS int
	var creatingNS int

	for _, ns := range nsList.Items {
		for _, owner := range ns.GetOwnerReferences() {
			if owner.UID == pool.GetUID() {
				switch ns.Annotations["pool-status"] {
				case "ready":
					readyNS++
				case "creating":
					creatingNS++
				case "error":
					r.Client.Delete(ctx, &ns)
				}
			}
		}
	}

	status := make(map[string]int)
	status["ready"] = readyNS
	status["creating"] = creatingNS

	return status, nil
}

func (r *PoolReconciler) underManaged(pool crd.Pool) bool {
	size := pool.Spec.Size
	ready := pool.Status.Ready
	creating := pool.Status.Creating

	if ready+creating < size {
		return true
	}
	return false
}

func (r *PoolReconciler) createNS(ctx context.Context, pool crd.Pool) error {
	// Create project or namespace depending on environment
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))
	r.Log.Info("Creating new namespace", "ns-name", ns.Name)

	initialAnnotations := map[string]string{
		"pool-status": "creating",
		"operator-ns": "true",
	}

	ns.SetOwnerReferences([]metav1.OwnerReference{pool.MakeOwnerReference()})

	if r.Config.PoolConfig.Local {
		if err := r.Client.Create(ctx, &ns); err != nil {
			return err
		}
	} else {
		project := projectv1.ProjectRequest{}
		project.Name = ns.Name
		if err := r.Client.Create(ctx, &project); err != nil {
			return err
		}
	}

	// Create ClowdEnvironment
	env := clowder.ClowdEnvironment{
		Spec: r.Config.ClowdEnvSpec,
	}
	env.SetName(fmt.Sprintf("env-%s", ns.Name))
	env.Spec.TargetNamespace = ns.Name

	// Retrieve namespace to populate APIVersion and Kind values
	// Use retry in case object retrieval is attempted before creation is done
	err := retry.OnError(
		wait.Backoff(retry.DefaultBackoff),
		func(error) bool { return true }, // hack - return true if err is notFound
		func() error {
			err := r.Client.Get(ctx, types.NamespacedName{Name: ns.Name}, &ns)
			return err
		},
	)
	if err != nil {
		r.Log.Error(err, "Cannot get namespace", "ns-name", ns.Name)
		return err
	}

	env.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: ns.APIVersion,
			Kind:       ns.Kind,
			Name:       ns.Name,
			UID:        ns.UID,
		},
	})

	r.Log.Info("Creating ClowdEnv for ns", "ns-name", ns.Name)
	if err := r.Client.Create(ctx, &env); err != nil {
		r.Log.Error(err, "Cannot Create ClowdEnv in Namespace", "ns-name", ns.Name)
		return err
	}

	// Set initial annotations on ns
	r.Log.Info("Setting initial annotations on ns", "ns-name", ns.Name)
	ns.SetAnnotations(initialAnnotations)
	err = r.Client.Update(ctx, &ns)
	if err != nil {
		r.Log.Error(err, "Could not update namespace annotations", "ns-name", ns.Name)
		return err
	}

	// Create LimitRange
	limitRange := r.Config.LimitRange
	limitRange.SetNamespace(ns.Name)
	if err := r.Client.Create(ctx, &limitRange); err != nil {
		r.Log.Error(err, "Cannot create LimitRange in Namespace", "ns-name", ns.Name)
		return err
	}

	// Create ResourceQuotas
	resourceQuotas := r.Config.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		quota.SetNamespace(ns.Name)
		if err := r.Client.Create(ctx, &quota); err != nil {
			r.Log.Error(err, "Cannot create ResourceQuota in Namespace", "ns-name", ns.Name)
			return err
		}
	}

	// Copy secrets
	secrets := core.SecretList{}
	err = r.Client.List(ctx, &secrets, client.InNamespace("ephemeral-base"))

	if err != nil {
		return err
	}

	r.Log.Info("Copying secrets from eph-base to new namespace", "ns-name", ns.Name)

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

		r.Log.Info("Copying secret", "secret-name", secret.Name, "ns-name", ns.Name)
		src := types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}

		dst := types.NamespacedName{
			Name:      secret.Name,
			Namespace: ns.Name,
		}

		err, newNsSecret := utils.CopySecret(ctx, r.Client, src, dst)
		if err != nil {
			r.Log.Error(err, "Unable to copy secret from source namespace")
			return err
		}

		if err := r.Client.Create(ctx, newNsSecret); err != nil {
			r.Log.Error(err, "Unable to apply secret from source namespace")
			return err
		}

	}

	return nil
}
