package controllers

import (
	"context"
	"fmt"
	"time"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	"github.com/go-logr/logr"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Poller struct {
	client.Client
	ActiveReservations map[string]metav1.Time
	Log                logr.Logger
}

const POLL_CYCLE time.Duration = 10

func (p *Poller) Poll() (ctrl.Result, error) {
	ctx := context.Background()
	p.Log.Info("Starting poller...")

	// Wait a period before beginning to poll
	// TODO workaround due to checking k8s objects too soon - revisit
	time.Sleep(time.Duration(30 * time.Second))

	p.Log.Info("Populating poller with active reservations")
	if err := p.populateActiveReservations(ctx); err != nil {
		p.Log.Error(err, "Unable to populate pool with active reservations")
		return ctrl.Result{}, err
	}

	for {
		// Check for expired reservations
		for k, v := range p.ActiveReservations {
			if p.namespaceIsExpired(v) {
				delete(p.ActiveReservations, k)

				res := crd.NamespaceReservation{}
				if err := p.Client.Get(ctx, types.NamespacedName{Name: k}, &res); err != nil {
					p.Log.Error(err, "Unable to retrieve reservation")
				}

				err := DeleteSubscriptionPrometheusOperator(ctx, p.Client, res.Status.Namespace)
				if k8serr.IsNotFound(err) {
					p.Log.Error(err, fmt.Sprintf("cannot find prometheus operator for namespace %s.", res.Status.Namespace))
				} else if err != nil {
					p.Log.Error(err, fmt.Sprintf("cannot delete prometheus operator subscription for namespace %s", res.Status.Namespace))
					return ctrl.Result{Requeue: true}, err
				} else {
					p.Log.Info("Successfully deleted", "prometheus-operator subscription", fmt.Sprint(res.Status.Namespace))
				}

				p.Log.Info("Reservation scheduled for deletion, deleting", "prometheus-operator", fmt.Sprintf("prometheus.%s", res.Status.Namespace))
				err = DeletePrometheusOperator(ctx, p.Client, res.Status.Namespace)
				if k8serr.IsNotFound(err) {
					p.Log.Error(err, fmt.Sprintf("the prometheus operator prometheus.%s does not exist.", res.Status.Namespace))
				} else if err != nil {
					p.Log.Error(err, fmt.Sprintf("Error deleting prometheus.%s", res.Status.Namespace))
					return ctrl.Result{Requeue: true}, err
				} else {
					p.Log.Info("Successfully deleted", "prometheus-operator", fmt.Sprintf("prometheus.%s", res.Status.Namespace))
				}

				if err := p.Client.Delete(ctx, &res); err != nil {
					p.Log.Error(err, "Unable to delete reservation", "ns-name", res.Status.Namespace)
				} else {
					p.Log.Info("Reservation for namespace has expired. Deleting.", "ns-name", res.Status.Namespace)
				}
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
