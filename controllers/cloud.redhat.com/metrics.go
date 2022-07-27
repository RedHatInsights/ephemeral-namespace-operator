package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	totalSuccessfulPoolReservationsCountMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "successful_pool_reservations_total",
			Help: "Total successful reservations from each pool",
		},
		[]string{"pool"},
	)

	totalFailedPoolReservationsCountMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "failed_pool_reservations_total",
			Help: "Total failed reservations from each pool",
		},
		[]string{"pool"},
	)

	requestedDurationTimeMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "namespace_reservation_duration_average",
			Help: "Average duration for namespace reservations",
			// Inf+ bucket is made implicitly by the prometheus library
			Buckets: []float64{1, 2, 4, 8, 24, 48, 168, 336},
		},
		[]string{"controller"},
	)

	namespaceCreationTimeMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "namespace_creation_time_minutes",
			Help:    "Average time namespace creation occurs'",
			Buckets: []float64{1, 2, 3, 4, 5, 7, 14, 28, 56, 112, 224},
		},
		[]string{"pool"},
	)

	averageReservationToDeploymentMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "reservation_to_deployment_time_seconds",
			Help:    "Average time it takes from reservation to deployment",
			Buckets: []float64{1, 2, 3, 4, 5, 7, 14, 28, 56, 112, 224},
		},
		[]string{"controller"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		totalSuccessfulPoolReservationsCountMetrics,
		totalFailedPoolReservationsCountMetrics,
		requestedDurationTimeMetrics,
		namespaceCreationTimeMetrics,
		averageReservationToDeploymentMetrics,
	)
}
