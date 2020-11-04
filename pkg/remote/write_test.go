package remote

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/pkg/context"
	"github.com/grafana/prometheus-pulsar-remote-write/pkg/pulsar"
)

type reqWithContext struct {
	req *prompb.WriteRequest
	ctx context.Context
}

type fakeWriteClient struct {
	t *testing.T

	errCh chan error
	reqCh chan reqWithContext
}

// Store stores the given samples in the remote storage.
func (f *fakeWriteClient) Store(ctx context.Context, compressed []byte) error {
	reqBuf, err := snappy.Decode(nil, compressed)
	require.NoError(f.t, err)

	var req prompb.WriteRequest

	err = req.Unmarshal(reqBuf)
	require.NoError(f.t, err)

	f.reqCh <- reqWithContext{
		req: &req,
		ctx: ctx,
	}

	return <-f.errCh
}

// Name uniquely identifies the remote storage.
func (f *fakeWriteClient) Name() string {
	panic("not implemented") // TODO: Implement
}

// Endpoint is the remote read or write endpoint for the storage client.
func (f *fakeWriteClient) Endpoint() string {
	panic("not implemented") // TODO: Implement
}

func newFakeWriteClient(t *testing.T) *fakeWriteClient {
	return &fakeWriteClient{
		t:     t,
		errCh: make(chan error),
		reqCh: make(chan reqWithContext),
	}
}

func withTenantID(s pulsar.ReceivedSample, tenantID string) pulsar.ReceivedSample {
	s.Context = mcontext.ContextWithTenantID(s.Context, tenantID)
	return s
}

func newBaseSample() pulsar.ReceivedSample {
	return pulsar.ReceivedSample{
		Ack: func() {
		},
		Nack: func() {
		},
		Context: context.Background(),
	}
}

// sample labels
var labelsMetric = model.Metric{
	model.MetricNameLabel: "foo",
	"labelfoo":            "label-bar",
}

var labelsPrompb = []prompb.Label{
	{
		Name:  model.MetricNameLabel,
		Value: "foo",
	},
	{
		Name:  "labelfoo",
		Value: "label-bar",
	},
}

var sampleNormal = prompb.Sample{
	Timestamp: 0,
	Value:     456,
}

func newSampleNormal() pulsar.ReceivedSample {
	s := newBaseSample()
	s.Sample = &model.Sample{
		Metric:    labelsMetric,
		Value:     model.SampleValue(sampleNormal.Value),
		Timestamp: model.Time(sampleNormal.Timestamp),
	}
	return s
}

var sampleInf = prompb.Sample{
	Value:     math.Inf(1),
	Timestamp: 10001,
}

func newSampleInf() pulsar.ReceivedSample {
	s := newBaseSample()
	s.Sample = &model.Sample{
		Metric:    labelsMetric,
		Value:     model.SampleValue(sampleInf.Value),
		Timestamp: model.Time(sampleInf.Timestamp),
	}
	return s
}

func TestWriter_Run_BatchSize_NoTenantID(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samplesCh := make(chan pulsar.ReceivedSample)

	w := NewWrite()
	wc := newFakeWriteClient(t)

	// reduce size to test
	w.BatchSize = 2
	// avoid hitting that
	w.BatchMaxDelay = time.Hour

	// watch channel
	go func() {
		err := w.Run(ctx, samplesCh, wc)
		require.NoError(t, err)
	}()

	// send two samples without channel ID
	samplesCh <- newSampleNormal()
	samplesCh <- newSampleInf()

	// retrieve request
	reqW := <-wc.reqCh
	wc.errCh <- nil

	req := reqW.req

	require.Equal(t, 2, len(req.Timeseries))

	// first sample
	assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
	assert.Equal(t, []prompb.Sample{sampleNormal}, req.Timeseries[0].Samples)

	// second sample
	assert.Equal(t, labelsPrompb, req.Timeseries[1].Labels)
	assert.Equal(t, []prompb.Sample{sampleInf}, req.Timeseries[1].Samples)
}

func TestWriter_Run_BatchSize_TenantIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samplesCh := make(chan pulsar.ReceivedSample)

	w := NewWrite()
	wc := newFakeWriteClient(t)

	// reduce size to test
	w.BatchSize = 2
	// avoid hitting that
	w.BatchMaxDelay = time.Hour

	// watch channel
	go func() {
		err := w.Run(ctx, samplesCh, wc)
		require.NoError(t, err)
	}()

	// send two samples per tenant ID
	go func() {
		samplesCh <- newSampleNormal()
		samplesCh <- withTenantID(newSampleNormal(), "team-a")
		samplesCh <- withTenantID(newSampleNormal(), "team-b")
		samplesCh <- newSampleInf()
		samplesCh <- withTenantID(newSampleInf(), "team-a")
		samplesCh <- withTenantID(newSampleInf(), "team-b")
	}()

	for _, tenantID := range []string{"", "team-a", "team-b"} {
		// retrieve request
		reqW := <-wc.reqCh
		wc.errCh <- nil

		req := reqW.req

		assert.Equal(t, tenantID, mcontext.TenantIDFromContext(reqW.ctx))

		require.Equal(t, 2, len(req.Timeseries))

		// first sample
		assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
		assert.Equal(t, []prompb.Sample{sampleNormal}, req.Timeseries[0].Samples)

		// second sample
		assert.Equal(t, labelsPrompb, req.Timeseries[1].Labels)
		assert.Equal(t, []prompb.Sample{sampleInf}, req.Timeseries[1].Samples)
	}
}
