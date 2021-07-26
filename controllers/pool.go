package controllers

import (
	"container/list"
	"context"
	"fmt"

	"strings"
	"time"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/utils"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// apps "k8s.io/api/apps/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	// "k8s.io/apimachinery/pkg/types"
	// "k8s.io/client-go/tools/record"
	// "k8s.io/client-go/util/workqueue"
)

const POLL_CYCLE time.Duration = 10
const POOL_DEPTH int = 5

type NamespacePool struct {
	ReadyNamespaces list.List
}

func (p *NamespacePool) AddOnDeckNS(ns string) {
	p.ReadyNamespaces.PushBack(ns)
}

func (p *NamespacePool) GetOnDeckNS() string {
	front := p.ReadyNamespaces.Front()
	p.ReadyNamespaces.Remove(front)
	return fmt.Sprintf("%s", front.Value)
}

func (p *NamespacePool) Len() int {
	return p.Len()
}

// Poll every POLL_CYCLE seconds to ensure there are a minimum number of ready namespaces
// and that expired namespaces are cleaned up
func Poll(client client.Client, pool *NamespacePool) error {
	ctx := context.Background()

	for {
		// Check for expired reservations
		// Ensure pool is desired size
		for pool.Len() < POOL_DEPTH {
			if err := pool.CreateOnDeckNamespace(ctx, client, pool); err != nil {
				return err
			}
		}
		// Check for reserved namespace expirations
		time.Sleep(time.Duration(POLL_CYCLE * time.Second))
	}
}

func (p *NamespacePool) CreateOnDeckNamespace(ctx context.Context, cl client.Client, pool *NamespacePool) error {
	// Create namespace
	ns := core.Namespace{}
	ns.Name = fmt.Sprintf("ephemeral-%s", strings.ToLower(randString(6)))
	err := cl.Create(ctx, &ns)

	if err != nil {
		return err
	}

	// Create ClowdEnvironment
	env := clowder.ClowdEnvironment{Spec: hardCodedEnvSpec()}
	env.SetName(ns.Name)
	env.Spec.TargetNamespace = ns.Name

	cl.Create(ctx, &env)

	// Copy secrets
	secrets := core.SecretList{}
	err = cl.List(ctx, &secrets, client.InNamespace("ephemeral-base"))

	if err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		src := types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}

		dst := types.NamespacedName{
			Name:      secret.Name,
			Namespace: ns.Name,
		}

		utils.CopySecret(ctx, cl, src, dst)
	}

	pool.AddOnDeckNS(ns.Name)

	return nil
}
