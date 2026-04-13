package agent

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ManagedPodsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "etmem_managed_pods_total",
		Help: "Number of Pods currently managed on this node",
	})
	ManagedProcessesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "etmem_managed_processes_total",
		Help: "Number of processes currently managed on this node",
	})
	ReconcileDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "etmem_reconcile_duration_seconds",
		Help:    "Time taken for a single reconcile loop",
		Buckets: prometheus.DefBuckets,
	})
	ReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "etmem_reconcile_errors_total",
		Help: "Total number of reconcile errors",
	})
	CircuitBreakerTrips = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "etmem_circuit_breaker_trips_total",
		Help: "Total number of circuit breaker trips",
	})
	SwappedBytesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "etmem_swapped_bytes",
		Help: "Total bytes currently swapped by etmem on this node",
	})
	TaskState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "etmem_task_state",
		Help: "Current state of each etmem task (1=active, 0=inactive)",
	}, []string{"project", "pod", "namespace", "state"})
)

func init() {
	metrics.Registry.MustRegister(
		ManagedPodsTotal,
		ManagedProcessesTotal,
		ReconcileDuration,
		ReconcileErrors,
		CircuitBreakerTrips,
		SwappedBytesTotal,
		TaskState,
	)
}
