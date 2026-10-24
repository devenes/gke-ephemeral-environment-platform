package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "preview_reconcile_total",
			Help: "Total number of PreviewEnvironment reconciliations partitioned by result.",
		},
		[]string{"result"},
	)

	EnvironmentsByPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "preview_environments_by_phase",
			Help: "Current number of PreviewEnvironment resources partitioned by lifecycle phase.",
		},
		[]string{"phase"},
	)

	ProvisioningDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "preview_provisioning_duration_seconds",
			Help:    "Duration in seconds from PreviewEnvironment creation to Ready phase.",
			Buckets: []float64{1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300},
		},
	)

	TTLCleanupDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "preview_ttl_cleanup_delay_seconds",
			Help:    "Delay in seconds between TTL expiration timestamp and controller cleanup initiation.",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
	)

	CleanupFailuresTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "preview_cleanup_failures_total",
			Help: "Total number of errors encountered during PreviewEnvironment teardown or finalizer cleanup.",
		},
	)
)

func init() {
	crmetrics.Registry.MustRegister(
		ReconcileTotal,
		EnvironmentsByPhase,
		ProvisioningDurationSeconds,
		TTLCleanupDelaySeconds,
		CleanupFailuresTotal,
	)
}
