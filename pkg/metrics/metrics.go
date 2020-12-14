package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	ReceivedSamples   *prometheus.CounterVec
	SentSamples       *prometheus.CounterVec
	FailedSamples     *prometheus.CounterVec
	RemoteRetries     *prometheus.CounterVec
	SentBatchDuration *prometheus.HistogramVec
}

func NewNopMetrics() *Metrics {
	return NewMetrics(nil)
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		ReceivedSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "received_samples_total",
				Help: "Total number of received samples.",
			},
			[]string{"tenant"},
		),
		SentSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "sent_samples_total",
				Help: "Total number of processed samples sent to remote storage.",
			},
			[]string{"remote", "tenant"},
		),
		FailedSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "failed_samples_total",
				Help: "Total number of processed samples which failed on send to remote storage.",
			},
			[]string{"remote", "tenant"},
		),
		RemoteRetries: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "retryable_failed_writes",
				Help: "Number of retryable failures when sending to remote storage",
			},
			[]string{"remote", "tenant"},
		),
		SentBatchDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "sent_batch_duration_seconds",
				Help:    "Duration of sample batch send calls to the remote storage.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"remote", "tenant"},
		),
	}
}
