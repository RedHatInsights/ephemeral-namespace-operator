package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	totalSuccessfulPoolReservationsCountMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "total_successful_pool_reservation_counter",
			Help: "Total successful reservations from each pool",
		},
		[]string{"pool"},
	)

	totalFailedPoolReservationsCountMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "total_failed_pool_reservation_counter",
			Help: "Total failed reservations from each pool",
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
		totalSuccessfulPoolReservationsCountMetrics,
		totalFailedPoolReservationsCountMetrics,
		averageRequestedDurationMetrics,
		averageReservationToDeploymentMetrics,
	)
}
