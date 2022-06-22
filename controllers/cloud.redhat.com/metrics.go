package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	totalPoolReservationsCountMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "total_pool_reservation_counter",
			Help: "Total reservations from each pool",
		},
		[]string{"pool"},
	)

	averageRequestedDurationMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "average_duration_for_namespace_reservations_in_hours",
			Help:    "Average duration for namespace reservations (In hours)",
			Buckets: []float64{1, 2, 4, 8, 24, 48, 168, 336},
		},
		[]string{"controller"},
	)

	averageReservationToDeploymentMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "average_time_from_reservation_to_deployment_in_milliseconds",
			Help:    "Average time it takes from reservation to deployment (In milliseconds)",
			Buckets: prometheus.LinearBuckets(5, 10, 10),
		},
		[]string{"controller"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		totalPoolReservationsCountMetrics,
		averageRequestedDurationMetrics,
		averageReservationToDeploymentMetrics,
	)
}
