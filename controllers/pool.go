package controllers

import (
	"container/list"
	"context"
	"fmt"
	"regexp"
	"sync"

	"errors"
	"strings"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"

	configv1 "github.com/openshift/api/config/v1"
	projectv1 "github.com/openshift/api/project/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const POLL_CYCLE time.Duration = 10

type NamespacePool struct {
	ReadyNamespaces    *list.List
	ActiveReservations map[string]metav1.Time
	Config             OperatorConfig
	Log                logr.Logger
}

func (p *NamespacePool) AddOnDeckNS(ns string) {
	p.ReadyNamespaces.PushBack(ns)
}

func (p *NamespacePool) GetOnDeckNs() string {
	front := p.ReadyNamespaces.Front()
	return fmt.Sprintf("%s", front.Value)
}

func (p *NamespacePool) CycleFrontToBack() {
	p.ReadyNamespaces.MoveToBack(p.ReadyNamespaces.Front())
}

func (p *NamespacePool) CheckoutNs(name string) error {
	for i := p.ReadyNamespaces.Front(); i != nil; i.Next() {
		stringName := fmt.Sprintf("%s", i.Value)
		if name == stringName {
			p.ReadyNamespaces.Remove(i)
			return nil
		}

	}
	errStr := fmt.Sprintf("Error, ns %s not found\n", name)
	return errors.New(errStr)
}

func (p *NamespacePool) Len() int {
	return p.ReadyNamespaces.Len()
}

// Poll every POLL_CYCLE seconds to ensure there are a minimum number of ready namespaces
// and that expired namespaces are cleaned up
func Poll(client client.Client, pool *NamespacePool) error {
	ctx := context.Background()
	pool.Log.Info("Starting pool...")

	// Wait a period before beginning to poll
	// TODO workaround due to checking k8s objects too soon - revisit
	time.Sleep(time.Duration(30 * time.Second))

	pool.Log.Info("Populating namespace list with existing namespaces")
	if err := pool.PopulateOnDeckNs(ctx, client); err != nil {
		pool.Log.Error(err, "Unable to populate namespace pool with existing namespaces: ")
		return err
	}

	pool.Log.Info("Populating pool with active reservations")
	if err := pool.populateActiveReservations(ctx, client); err != nil {
		pool.Log.Error(err, "Unable to populate pool with active reservations")
		return err
	}

	for {
		// Check for expired reservations
		for k, v := range pool.ActiveReservations {
			if pool.namespaceIsExpired(v) {
				delete(pool.ActiveReservations, k)

				res := crd.NamespaceReservation{}
				if err := client.Get(ctx, types.NamespacedName{Name: k}, &res); err != nil {
					pool.Log.Error(err, "Unable to retrieve reservation")
				}

				ns := core.Namespace{}
				err := client.Get(ctx, types.NamespacedName{Name: res.Status.Namespace}, &ns)
				if err != nil {
					pool.Log.Error(err, "Unable to retrieve namespace of expired reservation")
				} else {
					ns.Annotations["status"] = "deleting"
					err = client.Update(ctx, &ns)
					if err != nil {
						pool.Log.Error(err, "Could not update namespace", "ns-name", ns.Name)
					}

					err = client.Delete(ctx, &ns)
					if err != nil {
						pool.Log.Error(err, "Unable to delete namespace")
					}
				}

				res.Status.State = "expired"
				err = client.Status().Update(ctx, &res)
				if err != nil {
					pool.Log.Error(err, "Cannot update status")
				}
				pool.Log.Info("Reservation for namespace has expired. Deleting.", "ns-name", res.Status.Namespace)
			}
		}

		time.Sleep(time.Duration(POLL_CYCLE * time.Second))
	}
}

func (p *NamespacePool) PopulateOnDeckNs(ctx context.Context, client client.Client) error {
	nsList := core.NamespaceList{}
	if err := client.List(ctx, &nsList); err != nil {
		p.Log.Error(err, "Unable to retrieve list of existing ready namespaces")
		return err
	}

	for _, ns := range nsList.Items {
		matched, _ := regexp.MatchString(`ephemeral-\w{6}$`, ns.Name)
		if matched {
			if _, ok := ns.ObjectMeta.Annotations["reserved"]; !ok {
				ready, err := p.VerifyClowdEnv(ctx, client, ns)
				if ready {
					p.AddOnDeckNS(ns.Name)
					p.Log.Info("Added namespace to pool", "ns-name", ns.Name)
				} else {
					if err != nil {
						p.Log.Error(err, "Error retrieving clowdenv", "ns-name", ns.Name)
					} else {
						p.Log.Info("Existing namespace clowdenv is not ready. Recreating", "ns-name", ns.Name)
					}
					client.Delete(ctx, &ns)
				}
			}
		}
	}

	// Ensure pool is desired size at startup
	if p.Len() < p.Config.PoolConfig.Size {
		var wg sync.WaitGroup
		wg.Add(p.Config.PoolConfig.Size - p.Len())

		for i := p.Len(); i < p.Config.PoolConfig.Size; i++ {
			go func() {
				defer wg.Done()
				if err := p.CreateOnDeckNamespace(ctx, client); err != nil {
					p.Log.Error(err, "Unable to create on deck namespace")
				}
			}()
		}

		// Wait for pool to be filled
		wg.Wait()
	}

	return nil
}

func (p *NamespacePool) populateActiveReservations(ctx context.Context, client client.Client) error {
	resList, err := p.getExistingReservations(ctx, client)
	if err != nil {
		p.Log.Error(err, "Error retrieving list of reservations")
		return err
	}

	for _, res := range resList.Items {
		if res.Status.State == "active" {
			p.ActiveReservations[res.Name] = res.Status.Expiration
			p.Log.Info("Added active reservation to pool", "res-name", res.Name)
		}
	}

	return nil
}

func (p *NamespacePool) namespaceIsExpired(expiration metav1.Time) bool {
	remainingTime := expiration.Sub(time.Now())
	if !expiration.IsZero() && remainingTime <= 0 {
		return true
	}
	return false
}

func (p *NamespacePool) getExistingReservations(ctx context.Context, client client.Client) (*crd.NamespaceReservationList, error) {
	resList := crd.NamespaceReservationList{}
	err := client.List(ctx, &resList)
	if err != nil {
		p.Log.Error(err, "Cannot get reservations")
		return &resList, err
	}
	return &resList, nil

}

func (p *NamespacePool) getResFromNs(nsName string, resList *crd.NamespaceReservationList, ctx context.Context, client client.Client) (*crd.NamespaceReservation, error) {
	for _, res := range resList.Items {
		if res.Status.Namespace == nsName {
			return &res, nil
		}
	}
	errString := fmt.Sprintf("No reservation found for %s\n", nsName)
	return &crd.NamespaceReservation{}, errors.New(errString)
}

func (p *NamespacePool) VerifyClowdEnv(ctx context.Context, cl client.Client, ns core.Namespace) (bool, error) {
	env := clowder.ClowdEnvironment{}

	if err := cl.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("env-%s", ns.Name),
		Namespace: ns.Name,
	}, &env); err != nil {
		return false, err
	}

	conditions := env.Status.Conditions

	for i := range conditions {
		if conditions[i].Type == "DeploymentsReady" {
			if conditions[i].Status != "True" {
				return false, nil
			}
		}
	}

	return true, nil
}

func (p *NamespacePool) createFrontendEnv(ctx context.Context, cl client.Client, ns core.Namespace) error {
	ingressConfig := configv1.Ingress{}

	if !p.Config.PoolConfig.Local {
		// if on OpenShift, look up default ingress domain on cluster to set FrontendEnvironment's
		// hostname/sso attributes
		err := cl.Get(ctx, types.NamespacedName{Name: "cluster"}, &ingressConfig)
		if err != nil {
			p.Log.Error(err, "Unable to fetch 'config.ingresses' named 'cluster' to determine default domain")
			return err
		}
	}

	if ingressConfig.Spec.Domain == "" {
		// if no default domain, or if we're in local mode, just make use of a default local domain
		ingressConfig.Spec.Domain = "k8s.local"
	}

	p.Config.FrontendEnvSpec.Hostname = fmt.Sprintf("%s.%s", ns.Name, ingressConfig.Spec.Domain)
	p.Config.FrontendEnvSpec.SSO = fmt.Sprintf("https://%s/auth/", p.Config.FrontendEnvSpec.Hostname)

	frontendEnv := frontend.FrontendEnvironment{
		Spec: p.Config.FrontendEnvSpec,
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

	p.Log.Info("Creating FrontendEnvironment for ns", "ns-name", ns.Name)
	if err := cl.Create(ctx, &frontendEnv); err != nil {
		p.Log.Error(err, "Cannot create FrontendEnvironment for ns", "ns-name", ns.Name)
		return err
	}

	return nil
}

func (p *NamespacePool) CreateOnDeckNamespace(ctx context.Context, cl client.Client) error {
	// Create project or namespace depending on environment
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))
	p.Log.Info("Creating on deck namespace", "ns-name", ns.Name)

	initialAnnotations := map[string]string{
		"status":      "creating",
		"operator-ns": "true",
	}

	if p.Config.PoolConfig.Local {
		if err := cl.Create(ctx, &ns); err != nil {
			return err
		}
	} else {
		project := projectv1.ProjectRequest{}
		project.Name = ns.Name
		if err := cl.Create(ctx, &project); err != nil {
			return err
		}
	}

	// Create ClowdEnvironment
	env := clowder.ClowdEnvironment{
		Spec: p.Config.ClowdEnvSpec,
	}
	env.SetName(fmt.Sprintf("env-%s", ns.Name))
	env.Spec.TargetNamespace = ns.Name

	// Retrieve namespace to populate APIVersion and Kind values
	// Use retry in case object retrieval is attempted before creation is done
	err := retry.OnError(
		wait.Backoff(retry.DefaultBackoff),
		func(error) bool { return true }, // hack - return true if err is notFound
		func() error {
			err := cl.Get(ctx, types.NamespacedName{Name: ns.Name}, &ns)
			return err
		},
	)
	if err != nil {
		p.Log.Error(err, "Cannot get namespace", "ns-name", ns.Name)
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

	p.Log.Info("Creating ClowdEnv for ns", "ns-name", ns.Name)
	if err := cl.Create(ctx, &env); err != nil {
		p.Log.Error(err, "Cannot Create ClowdEnv in Namespace", "ns-name", ns.Name)
		return err
	}

	// Create FrontendEnvironment
	if err := p.createFrontendEnv(ctx, cl, ns); err != nil {
		return err
	}

	// Set initial annotations on ns
	p.Log.Info("Setting initial annotations on ns", "ns-name", ns.Name)
	ns.SetAnnotations(initialAnnotations)
	err = cl.Update(ctx, &ns)
	if err != nil {
		p.Log.Error(err, "Could not update namespace annotations", "ns-name", ns.Name)
		return err
	}

	// Create LimitRange
	limitRange := p.Config.LimitRange
	limitRange.SetNamespace(ns.Name)
	if err := cl.Create(ctx, &limitRange); err != nil {
		p.Log.Error(err, "Cannot create LimitRange in Namespace", "ns-name", ns.Name)
		return err
	}

	// Create ResourceQuotas
	resourceQuotas := p.Config.ResourceQuotas
	for _, quota := range resourceQuotas.Items {
		quota.SetNamespace(ns.Name)
		if err := cl.Create(ctx, &quota); err != nil {
			p.Log.Error(err, "Cannot create ResourceQuota in Namespace", "ns-name", ns.Name)
			return err
		}
	}

	// Copy secrets
	secrets := core.SecretList{}
	err = cl.List(ctx, &secrets, client.InNamespace("ephemeral-base"))

	if err != nil {
		return err
	}

	p.Log.Info("Copying secrets from eph-base to new namespace", "ns-name", ns.Name)

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

		p.Log.Info("Copying secret", "secret-name", secret.Name, "ns-name", ns.Name)
		src := types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}

		dst := types.NamespacedName{
			Name:      secret.Name,
			Namespace: ns.Name,
		}

		err, newNsSecret := utils.CopySecret(ctx, cl, src, dst)
		if err != nil {
			p.Log.Error(err, "Unable to copy secret from source namespace")
			return err
		}

		if err := cl.Create(ctx, newNsSecret); err != nil {
			p.Log.Error(err, "Unable to apply secret from source namespace")
			return err
		}

	}

	// TODO: revisit this check
	// We need to wait a bit before checking the clowdEnv
	p.Log.Info("Verifying that the ClowdEnv is ready for namespace", "ns-name", ns.Name)
	time.Sleep(10 * time.Second)

	ready, _ := p.VerifyClowdEnv(ctx, cl, ns)
	for !ready {
		p.Log.Info("Waiting on environment to be ready", "ns-name", ns.Name)
		time.Sleep(10 * time.Second)
		ready, _ = p.VerifyClowdEnv(ctx, cl, ns)
	}

	err = cl.Get(ctx, types.NamespacedName{Name: ns.Name}, &ns)
	if err != nil {
		p.Log.Error(err, "Unable to retrieve namespace to update annotations")
	}

	ns.Annotations["status"] = "ready"
	err = cl.Update(ctx, &ns)
	if err != nil {
		p.Log.Error(err, "Could not update namespace", "ns-name", ns.Name)
		return err
	}

	p.AddOnDeckNS(ns.Name)
	p.Log.Info("Namespace added to the ready pool", "ns-name", ns.Name)

	return nil
}
