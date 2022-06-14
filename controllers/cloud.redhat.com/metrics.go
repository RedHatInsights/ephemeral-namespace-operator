package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	totalReservationCountMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "total_reservation_counter",
			Help: "Total reservations",
		},
	)

	totalDefaultPoolReservationsCountMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "total_default_pool_reservation_counter",
			Help: "Total reservations from the 'default' pool",
		},
	)

	totalMinimalPoolReservationsCountMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "total_minimal_pool_reservation_counter",
			Help: "Total reservations from the 'minimal' pool",
		},
	)

	totalManagedKafkaPoolReservationsCountMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "total_managed_kafka_pool_reservation_counter",
			Help: "Total reservations from the 'managed-kafka' pool",
		},
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
			Buckets: prometheus.LinearBuckets(200, 10, 10),
		},
		[]string{"controller"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		totalReservationCountMetrics,
		totalDefaultPoolReservationsCountMetrics,
		totalMinimalPoolReservationsCountMetrics,
		totalManagedKafkaPoolReservationsCountMetrics,
		averageRequestedDurationMetrics,
		averageReservationToDeploymentMetrics,
	)
}
