package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type ConsumerMetrics struct {
	WrittenSamples            prometheus.Counter
	RecoverableWriteErrors    *prometheus.CounterVec
	NonRecoverableWriteErrors *prometheus.CounterVec
	WriteBatchDuration        *prometheus.HistogramVec
}

func NewConsumerMetricsDefault() *ConsumerMetrics {
	return newConsumerMetricsFromRegistry(nil)
}

func NewConsumerMetrics(r prometheus.Registerer) *ConsumerMetrics {
	return newConsumerMetricsFromRegistry(r)
}

func newConsumerMetricsFromRegistry(r prometheus.Registerer) *ConsumerMetrics {
	return &ConsumerMetrics{
		WrittenSamples: promauto.With(r).NewCounter(
			prometheus.CounterOpts{
				Name: "written_samples_total",
				Help: "Total number of samples written to remote storage",
			},
		),
		RecoverableWriteErrors: promauto.With(r).NewCounterVec(
			prometheus.CounterOpts{
				Name: "recoverable_remote_write_errors",
				Help: "Recoverable errors encountered writing to remote storage",
			},
			[]string{"tenant"},

		),
		NonRecoverableWriteErrors: promauto.With(r).NewCounterVec(
			prometheus.CounterOpts{
				Name: "non_recoverable_remote_write_errors",
				Help: "Non-recoverable errors encountered writing to remote storage",
			},
			[]string{"tenant"},

		),
		WriteBatchDuration: promauto.With(r).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "remote_write_batch_duration_seconds",
				Help:    "Duration of send calls to remote storage",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"tenant"},
		),
	}
}
