package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var durationQueue = make([]float64, 0)
var averageDurationRequested float64

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

	averageRequestedDurationMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "average_duration_for_namespace_reservations_in_hours",
			Help: "Average duration for namespace reservations in hours (Last 500)",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		totalReservationCountMetrics,
		totalDefaultPoolReservationsCountMetrics,
		totalMinimalPoolReservationsCountMetrics,
		totalManagedKafkaPoolReservationsCountMetrics,
		averageRequestedDurationMetrics,
	)
}
