package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	totalFailedPoolReservationsCountMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "failed_pool_reservations_total",
			Help: "Total failed reservations from each pool",
		},
		[]string{"pool"},
	)

	averageRequestedDurationMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "namespace_reservation_duration_average",
			Help: "Average duration for namespace reservations (In hours)",
			// Inf+ bucket is made implicitly by the prometheus library
			Buckets: []float64{1, 2, 4, 8, 24, 48, 168, 336},
		},
		[]string{"controller", "pool"},
	)

	averageNamespaceCreationMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "namespace_creation_average_seconds",
			Help:    "Average time namespace creation occurs'",
			Buckets: []float64{1, 2, 3, 4, 5, 7, 14, 28, 56, 112, 224},
		},
		[]string{"pool"},
	)

	averageReservationToDeploymentMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "average_time_reservation_to_deployment_seconds",
			Help:    "Average time it takes from reservation to deployment in milliseconds",
			Buckets: []float64{1, 2, 3, 4, 5, 7, 14, 28, 56, 112, 224},
		},
		[]string{"controller", "pool"},
	)

	activeReservationTotalMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_reservation_total",
			Help: "Total active reservations",
		},
		[]string{"controller"},
	)

	enoVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "eno_version",
			Help: "ENOVersion 1 if present, 0 if not",
		},
		[]string{"version"},
	)

	capiCleanupDurationMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cleanup_duration_seconds",
			Help: "Time elapsed since namespace entered CAPI cleanup state (deletion timestamp to now)",
		},
		[]string{"namespace", "reservation"},
	)

	reservationsByRequesterTotalMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reservations_by_requester_total",
			Help: "Total number of namespace reservations by requester and pool",
		},
		[]string{"requester", "pool"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		totalFailedPoolReservationsCountMetrics,
		averageRequestedDurationMetrics,
		averageNamespaceCreationMetrics,
		averageReservationToDeploymentMetrics,
		activeReservationTotalMetrics,
		enoVersion,
		capiCleanupDurationMetrics,
		reservationsByRequesterTotalMetrics,
	)
}
