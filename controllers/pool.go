package controllers

import (
	"container/list"
	"context"
	"fmt"
	"regexp"

	"errors"
	"strings"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"

	//k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// apps "k8s.io/api/apps/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	// "k8s.io/apimachinery/pkg/types" "k8s.io/client-go/tools/record" "k8s.io/client-go/util/workqueue"
)

const POLL_CYCLE time.Duration = 10
const POOL_DEPTH int = 1

type NamespacePool struct {
	ReadyNamespaces *list.List
	Log             logr.Logger
}

func (p *NamespacePool) AddOnDeckNS(ns string) {
	p.ReadyNamespaces.PushBack(ns)
}

func (p *NamespacePool) GetOnDeckNs() string {
	front := p.ReadyNamespaces.Front()
	return fmt.Sprintf("%s", front.Value)
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

	if err := pool.populateOnDeckNs(ctx, client); err != nil {
		fmt.Println("Unable to populate namespace pool with existing namespaces: ", err)
		return err
	}

	for {
		// Check for expired reservations
		// First pass is very unoptimized; this is O(n) every 10s
		// We can add the expiration time to the pool and check that as a map
		// Or we can investiage time.After() for each namespace
		resList, err := pool.getExistingReservations(client, ctx)
		if err != nil {
			fmt.Println("Unable to retrieve list of active reservations")
			return err
		}

		for _, res := range resList.Items {
			if pool.namespaceIsExpired(&res) {
				err := client.Delete(ctx, &res)
				if err != nil {
					fmt.Println("Unable to delete expired reservation")
					return err
				}
				fmt.Printf("Reservation for namespace %s has expired. Deleting.\n", res.Status.Namespace)
			}
		}

		// Check for reserved namespace expirations
		time.Sleep(time.Duration(POLL_CYCLE * time.Second))
	}
}

func (p *NamespacePool) populateOnDeckNs(ctx context.Context, client client.Client) error {
	nsList := core.NamespaceList{}

	// TODO: Revisit method of determining cache readiness
	// Cannot retrieve list of namespaces right away
	// TODO: Max retries or timeout waiting for cache
	cacheReady := false
	for !cacheReady {
		if err := client.List(ctx, &nsList); err == nil {
			cacheReady = true
		}
		time.Sleep(time.Duration(2 * time.Second))
	}

	for _, ns := range nsList.Items {
		matched, _ := regexp.MatchString(`ephemeral-\w{6}`, ns.Name)
		if matched {
			if _, ok := ns.ObjectMeta.Annotations["reserved"]; !ok {
				p.AddOnDeckNS(ns.Name)
			}
		}
	}

	// Ensure pool is desired size
	for p.Len() < POOL_DEPTH {
		if err := p.CreateOnDeckNamespace(ctx, client); err != nil {
			return err
		}

	}

	return nil
}

func (p *NamespacePool) namespaceIsExpired(res *crd.NamespaceReservation) bool {
	remainingTime := res.Status.Expiration.Sub(time.Now())
	if remainingTime <= 0 {
		return true
	}
	return false
}

func (p *NamespacePool) getExistingReservations(client client.Client, ctx context.Context) (*crd.NamespaceReservationList, error) {
	resList := crd.NamespaceReservationList{}
	err := client.List(ctx, &resList)
	if err != nil {
		fmt.Println("Cannot get reservations")
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

func (p *NamespacePool) CreateOnDeckNamespace(ctx context.Context, cl client.Client) error {
	// Create namespace
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))
	fmt.Printf("Creating on deck ns for %s\n", ns.Name)
	err := cl.Create(ctx, &ns)

	if err != nil {
		return err
	}

	// Create ClowdEnvironment
	env := clowder.ClowdEnvironment{
		Spec: hardCodedEnvSpec(),
	}
	env.SetName(ns.Name)
	env.Spec.TargetNamespace = ns.Name

	if err := cl.Create(ctx, &env); err != nil {
		fmt.Printf("Cannot Create ClowdEnv in Namespace %s\n", ns.Name)
		fmt.Printf("Error: %s", err)
		return err
	}

	// Copy secrets
	secrets := core.SecretList{}
	err = cl.List(ctx, &secrets, client.InNamespace("ephemeral-base"))

	if err != nil {
		return err
	}

	fmt.Printf("Copying secrets from eph-base\n")

	for _, secret := range secrets.Items {
		if strings.Contains(secret.Name, "default-token") {
			continue
		}
		fmt.Printf("Copying secret %s\n", secret.Name)
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
			fmt.Printf("Unable to copy secret from source namespace: %s\n", err)
			return err
		}

		if err := cl.Create(ctx, newNsSecret); err != nil {
			fmt.Printf("Unable to apply secret from source namespace: %s\n", err)
			return err
		}

	}

	fmt.Printf("Verifying that the ClowdEnv is ready for ns %s\n", ns.Name)
	for !env.IsReady() {
		time.Sleep(2 * time.Second)
	}
	p.AddOnDeckNS(ns.Name)
	fmt.Printf("Namespace %s added to the pool\n", ns.Name)

	return nil
}
