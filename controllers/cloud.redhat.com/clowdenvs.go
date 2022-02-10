package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateClowdEnv(ctx context.Context, cl client.Client, spec clowder.ClowdEnvironmentSpec, nsName string) error {
	env := clowder.ClowdEnvironment{
		Spec: spec,
	}
	env.SetName(fmt.Sprintf("env-%s", nsName))
	env.Spec.TargetNamespace = nsName

	ns, err := GetNamespace(ctx, cl, nsName)
	if err != nil {
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

	if err := cl.Create(ctx, &env); err != nil {
		return err
	}

	return nil
}

func WaitForClowdEnv(ctx context.Context, cl client.Client, log logr.Logger, nsName string) *clowder.ClowdEnvironment {
	var clowdEnv *clowder.ClowdEnvironment
	var ready bool
	var err error
	for {
		time.Sleep(10 * time.Second)

		ready, clowdEnv, err = GetClowdEnv(ctx, cl, nsName)
		if ready && clowdEnv != nil {
			break
		}

		// env is not ready
		msg := "ClowdEnvironment is not yet ready for namespace"
		if err != nil {
			msg = msg + fmt.Sprintf(" (%s)", err)
		}
		log.Info(msg, "ns-name", nsName)
	}

	return clowdEnv
}

func GetClowdEnv(ctx context.Context, cl client.Client, nsName string) (bool, *clowder.ClowdEnvironment, error) {
	env := clowder.ClowdEnvironment{}
	nn := types.NamespacedName{
		Name:      fmt.Sprintf("env-%s", nsName),
		Namespace: nsName,
	}

	err := cl.Get(ctx, nn, &env)
	if err != nil {
		return false, nil, err
	}

	ready, err := VerifyClowdEnvReady(env)

	return ready, &env, err
}

func VerifyClowdEnvReady(env clowder.ClowdEnvironment) (bool, error) {
	// check that hostname is populated if ClowdEnvironment is operating in 'local' web mode
	if env.Spec.Providers.Web.Mode == "local" && env.Status.Hostname == "" {
		return false, errors.New("hostname not populated")
	}

	// check that all deployments are ready
	conditions := env.Status.Conditions
	reconciliationSuccessful := false
	deploymentsReady := false
	for i := range conditions {
		if conditions[i].Type == "ReconciliationSuccessful" && conditions[i].Status == "True" {
			reconciliationSuccessful = true
		}
		if conditions[i].Type == "DeploymentsReady" && conditions[i].Status == "True" {
			deploymentsReady = true
		}
	}

	if !reconciliationSuccessful {
		return false, errors.New("reconciliation not successful")
	}
	if !deploymentsReady {
		return false, errors.New("deployments not ready")
	}

	return true, nil
}
