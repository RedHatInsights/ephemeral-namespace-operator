package controllers

import (
	"context"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Poller struct {
	client             client.Client
	activeReservations map[string]metav1.Time
	log                logr.Logger
}

const PollCycle time.Duration = 10

func (p *Poller) Poll() {
	ctx := context.Background()
	p.log.Info("Starting poller...")

	// Wait a period before beginning to poll
	// TODO workaround due to checking k8s objects too soon - revisit
	time.Sleep(time.Duration(30 * time.Second))

	p.log.Info("Populating poller with active reservations")
	if err := p.populateActiveReservations(ctx); err != nil {
		p.log.Error(err, "Unable to populate pool with active reservations")
		return
	}

	for {
		// Check for expired reservations
		for k, v := range p.activeReservations {
			if p.namespaceIsExpired(v) {
				delete(p.activeReservations, k)

				res := crd.NamespaceReservation{}
				if err := p.client.Get(ctx, types.NamespacedName{Name: k}, &res); err != nil {
					p.log.Error(err, "Unable to retrieve reservation")
				}

				if err := p.client.Delete(ctx, &res); err != nil {
					p.log.Error(err, "Unable to delete reservation", "namespace", res.Status.Namespace)
				} else {
					p.log.Info("deleting expired reservation", "namespace", res.Status.Namespace)
				}

				activeReservationTotalMetrics.With(prometheus.Labels{"controller": "namespacereservation"}).Set(float64(len(p.activeReservations)))
			}
		}

		time.Sleep(time.Duration(PollCycle * time.Second))
	}
}

func (p *Poller) populateActiveReservations(ctx context.Context) error {
	resList, err := p.getExistingReservations(ctx)
	if err != nil {
		p.log.Error(err, "Error retrieving list of reservations")
		return err
	}

	for _, res := range resList.Items {
		if res.Status.State == "active" {
			p.activeReservations[res.Name] = res.Status.Expiration
			p.log.Info("Added active reservation to poller", "res-name", res.Name)
		}
	}

	return nil
}

func (p *Poller) getExistingReservations(ctx context.Context) (*crd.NamespaceReservationList, error) {
	resList := crd.NamespaceReservationList{}
	err := p.client.List(ctx, &resList)
	if err != nil {
		p.log.Error(err, "Cannot get reservations")
		return &resList, err
	}
	return &resList, nil

}

func (p *Poller) namespaceIsExpired(expiration metav1.Time) bool {
	remainingTime := time.Until(expiration.Time)
	if !expiration.IsZero() && remainingTime <= 0 {
		return true
	}
	return false
}
