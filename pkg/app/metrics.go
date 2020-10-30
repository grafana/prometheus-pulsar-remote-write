package app

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	receivedSamples   prometheus.Counter
	sentSamples       *prometheus.CounterVec
	failedSamples     *prometheus.CounterVec
	sentBatchDuration *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	return &metrics{
		receivedSamples: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "received_samples_total",
				Help: "Total number of received samples.",
			},
		),
		sentSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "sent_samples_total",
				Help: "Total number of processed samples sent to remote storage.",
			},
			[]string{"remote"},
		),
		failedSamples: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "failed_samples_total",
				Help: "Total number of processed samples which failed on send to remote storage.",
			},
			[]string{"remote"},
		),
		sentBatchDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "sent_batch_duration_seconds",
				Help:    "Duration of sample batch send calls to the remote storage.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"remote"},
		),
	}
}
