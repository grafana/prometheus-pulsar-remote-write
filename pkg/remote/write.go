package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/pkg/context"
	"github.com/grafana/prometheus-pulsar-remote-write/pkg/pulsar"
	"github.com/grafana/prometheus-pulsar-remote-write/pkg/version"
)

type Write struct {
	logger log.Logger

	samplePerTenantID   map[string][]pulsar.ReceivedSample
	deadlinePerTenantID map[string]time.Time

	// BatchSize is the amount of samples that are aggregated into a single
	// remote write request
	BatchSize int
	// BatchMaxDelay is the maximum delay acceptable for a batch to wait to
	// reach the BatchSize
	BatchMaxDelay time.Duration

	// checkInterval limits how often we want to check on send conditions for batches are met
	checkInterval time.Duration
}

type WriteOpts func(o *Write)

func WithLogger(l log.Logger) WriteOpts {
	return func(w *Write) {
		w.logger = l
	}
}

func NewWrite(opts ...WriteOpts) *Write {
	w := &Write{
		logger: log.NewNopLogger(),

		samplePerTenantID:   make(map[string][]pulsar.ReceivedSample),
		deadlinePerTenantID: make(map[string]time.Time),

		BatchSize:     100,
		BatchMaxDelay: 5 * time.Second,
		checkInterval: 100 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

type headerRoundtripper struct {
	upstream http.RoundTripper
}

func (t *headerRoundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// if tenant ID is set, expose it in the header
	tenantID := mcontext.TenantIDFromContext(req.Context())
	if tenantID != "" {
		req.Header.Set(mcontext.HTTPHeaderTenantID, tenantID)
	}

	// overwrite the user agent header
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", version.AppName(), version.AppVersion()))

	return t.upstream.RoundTrip(req)
}

type ClientConfig = remote.ClientConfig

func NewWriteClient(conf *ClientConfig) (remote.WriteClient, error) {
	clientInterface, err := remote.NewWriteClient("remote-write", conf)
	if err != nil {
		return nil, err
	}

	client, ok := clientInterface.(*remote.Client)
	if !ok {
		return nil, fmt.Errorf("client of unexpected type %T", clientInterface)
	}

	// add custom rountripper to modify user-agent and tenant header if necessary
	client.Client.Transport = &headerRoundtripper{upstream: client.Client.Transport}

	return client, nil
}

func (w *Write) Run(ctx context.Context, sampleCh chan pulsar.ReceivedSample, client remote.WriteClient) error {
	tick := time.NewTicker(w.checkInterval)
	defer tick.Stop()

	// this is set when a retry able error happend
	errRemoteWriteRetryable := false
	blockingSampleCh := make(chan pulsar.ReceivedSample)

	sample := func() chan pulsar.ReceivedSample {
		// don't return new samples if err has happend
		if errRemoteWriteRetryable {
			return blockingSampleCh
		}
		return sampleCh
	}

	var samplesSinceCheck int
	var deadlineCheck time.Time

receive:

	for {
		// wait for either tick or incoming sample, handle closed context
		select {
		case <-ctx.Done():
			break receive
		case <-tick.C:
			// do nothing
		case sample := <-sample():
			tenantID := mcontext.TenantIDFromContext(sample.Context)
			if _, ok := w.samplePerTenantID[tenantID]; !ok {
				w.samplePerTenantID[tenantID] = []pulsar.ReceivedSample{}
				w.deadlinePerTenantID[tenantID] = time.Now().Add(w.BatchMaxDelay)
			}

			w.samplePerTenantID[tenantID] = append(w.samplePerTenantID[tenantID],
				sample,
			)
			samplesSinceCheck += 1
		}

		// continue reading samples from channel, if no errors has happened, we
		// didn't hit the check deadline and the samples received are smaller
		// than the BatchSize:
		if !errRemoteWriteRetryable &&
			time.Now().Before(deadlineCheck) &&
			samplesSinceCheck < w.BatchSize {
			continue
		}

		// reset check conditions
		errRemoteWriteRetryable = false
		samplesSinceCheck = 0
		deadlineCheck = time.Now().Add(w.checkInterval)

		// loop through tenants and find metrics to send
		for tenantID, samples := range w.samplePerTenantID {
			logger := tenantIDLogger(w.logger, tenantID)

			batchSizeReached := len(samples) >= w.BatchSize
			pastDeadline := w.deadlinePerTenantID[tenantID].Before(time.Now())

			if !batchSizeReached && !pastDeadline {
				continue
			}

			req := samplesToProto(samples)

			data, err := proto.Marshal(req)
			if err != nil {
				return err
			}

			compressed := snappy.Encode(nil, data)

			if err := client.Store(mcontext.ContextWithTenantID(ctx, tenantID), compressed); err != nil {
				errRec := &remote.RecoverableError{}
				if errors.As(err, errRec) {
					_ = level.Warn(w.logger).Log("msg", "failed remote_write request, will retry", "error", err)
					errRemoteWriteRetryable = true
					continue
				}

				// non-recoverable errors still require all messages to be
				// acked, as we otherwise would get them redelivered through
				// pulsar
				_ = level.Error(w.logger).Log("msg", "failed remote_write reqeust", "error", err)

			} else {
				_ = level.Debug(logger).Log(
					"msg", "remote_write request succesful",
					"sample_count", len(samples),
				)
			}

			// Ack the messages
			for _, s := range samples {
				s.Ack()
			}

			delete(w.samplePerTenantID, tenantID)
			delete(w.deadlinePerTenantID, tenantID)
		}

	}

	return nil
}

func tenantIDLogger(l log.Logger, tenantID string) log.Logger {
	if tenantID == "" {
		return l
	}
	return log.With(l, "tenant_id", tenantID)
}

func metricToProtoLabel(m model.Metric) []prompb.Label {
	labels := make([]prompb.Label, len(m))

	var names []string
	for name := range m {
		names = append(names, string(name))
	}
	sort.Strings(names)

	for pos, name := range names {
		labels[pos] = prompb.Label{
			Name:  name,
			Value: string(m[model.LabelName(name)]),
		}
	}
	return labels
}

func samplesToProto(samples []pulsar.ReceivedSample) *prompb.WriteRequest {
	var req prompb.WriteRequest

	req.Timeseries = make([]prompb.TimeSeries, len(samples))
	for pos, sample := range samples {
		req.Timeseries[pos] = prompb.TimeSeries{
			Labels: metricToProtoLabel(sample.Sample.Metric),
			Samples: []prompb.Sample{{
				Value:     float64(sample.Sample.Value),
				Timestamp: int64(sample.Sample.Timestamp),
			}},
		}
	}

	return &req
}
