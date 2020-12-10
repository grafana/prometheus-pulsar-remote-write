package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type ProducerMetrics struct {
	ReceivedSamples   prometheus.Counter
	SentSamples       *prometheus.CounterVec
	FailedSamples     *prometheus.CounterVec
	SentBatchDuration *prometheus.HistogramVec
}

func NewProducerMetrics(reg prometheus.Registerer) *ProducerMetrics {
	return &ProducerMetrics{
		ReceivedSamples: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "received_samples_total",
				Help: "Total number of received samples.",
			},
		),
		SentSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "sent_samples_total",
				Help: "Total number of processed samples sent to remote storage.",
			},
			[]string{"remote"},
		),
		FailedSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "failed_samples_total",
				Help: "Total number of processed samples which failed on send to remote storage.",
			},
			[]string{"remote"},
		),
		SentBatchDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "sent_batch_duration_seconds",
				Help:    "Duration of sample batch send calls to the remote storage.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"remote"},
		),
	}
}
