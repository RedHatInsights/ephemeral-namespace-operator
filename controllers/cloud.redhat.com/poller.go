package controllers

import (
	"context"
	"fmt"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Poller struct {
	client.Client
	ActiveReservations map[string]metav1.Time
	Log                logr.Logger
}

const POLL_CYCLE time.Duration = 10

func (p *Poller) Poll() error {
	ctx := context.Background()
	p.Log.Info("Starting poller...")

	// Wait a period before beginning to poll
	// TODO workaround due to checking k8s objects too soon - revisit
	time.Sleep(time.Duration(30 * time.Second))

	p.Log.Info("Populating poller with active reservations")
	if err := p.populateActiveReservations(ctx); err != nil {
		p.Log.Error(err, "Unable to populate pool with active reservations")
		return err
	}

	for {
		// Check for expired reservations
		for k, v := range p.ActiveReservations {
			if p.namespaceIsExpired(v) {
				res := crd.NamespaceReservation{}
				if err := p.Client.Get(ctx, types.NamespacedName{Name: k}, &res); err != nil {
					p.Log.Error(err, "Unable to retrieve reservation")
				}

				if res.Status.Namespace != "" {
					removed, err := CheckForSubscriptionPrometheusOperator(ctx, p.Client, res.Status.Namespace)
					if !removed {
						p.Log.Error(fmt.Errorf("waiting for subscription to be deleted from [%s]", res.Status.Namespace), "subscription still exists")
						continue
					} else if err != nil {
						p.Log.Error(err, "error checking for subscription [%s]", res.Status.Namespace)
						continue
					}

					_, err = DeletePrometheusOperator(ctx, p.Client, p.Log, res.Status.Namespace)
					if err != nil {
						p.Log.Error(err, "deletion of prometheus operator was unsuccesful")
						continue
					}
				}

				if err := p.Client.Delete(ctx, &res); err != nil {
					p.Log.Error(err, "Unable to delete reservation", "ns-name", res.Status.Namespace)
				} else {
					p.Log.Info("Reservation for namespace has expired. Deleting.", "ns-name", res.Status.Namespace)
				}

				delete(p.ActiveReservations, k)
			}
		}

		time.Sleep(time.Duration(POLL_CYCLE * time.Second))
	}
}

func (p *Poller) populateActiveReservations(ctx context.Context) error {
	resList, err := p.getExistingReservations(ctx)
	if err != nil {
		p.Log.Error(err, "Error retrieving list of reservations")
		return err
	}

	for _, res := range resList.Items {
		if res.Status.State == "active" {
			p.ActiveReservations[res.Name] = res.Status.Expiration
			p.Log.Info("Added active reservation to poller", "res-name", res.Name)
		}
	}

	return nil
}

func (p *Poller) getExistingReservations(ctx context.Context) (*crd.NamespaceReservationList, error) {
	resList := crd.NamespaceReservationList{}
	err := p.Client.List(ctx, &resList)
	if err != nil {
		p.Log.Error(err, "Cannot get reservations")
		return &resList, err
	}
	return &resList, nil

}

func (p *Poller) namespaceIsExpired(expiration metav1.Time) bool {
	remainingTime := expiration.Sub(time.Now())
	if !expiration.IsZero() && remainingTime <= 0 {
		return true
	}
	return false
}
