package remote

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
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
	t testing.TB

	benchmark bool

	errCh chan error
	reqCh chan reqWithContext
}

// Store stores the given samples in the remote storage.
func (f *fakeWriteClient) Store(ctx context.Context, compressed []byte) error {
	if f.benchmark {
		return nil
	}
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

func newFakeWriteClient(t testing.TB) *fakeWriteClient {
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

type runTestCase struct {
	name   string
	send   func(chan pulsar.ReceivedSample)
	verify func(testing.TB, *fakeWriteClient)

	batchMaxDelay time.Duration
	batchSize     int
}

func (tc runTestCase) test(t *testing.T) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samplesCh := make(chan pulsar.ReceivedSample)

	w := NewWrite()
	wc := newFakeWriteClient(t)

	w.checkInterval = time.Microsecond
	if tc.batchSize > 0 {
		w.BatchSize = tc.batchSize

	} else {
		// reduce size to test
		w.BatchSize = 2
	}
	if tc.batchMaxDelay > 0 {
		w.BatchMaxDelay = tc.batchMaxDelay
	} else {
		// set very high value, to avoid hitting that in the default case
		w.BatchMaxDelay = time.Hour
	}

	// watch channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.Run(ctx, samplesCh, wc)
		require.NoError(t, err)
	}()

	// send samples
	wg.Add(1)
	go func() {
		defer wg.Done()
		tc.send(samplesCh)
	}()

	// retrieve request
	tc.verify(t, wc)

	cancel()
	wg.Wait()

}

func testForTenantIDs(t testing.TB, wc *fakeWriteClient, tenantIDSlice []string, verify func(testing.TB, *prompb.WriteRequest)) {
	tenantIDs := make(map[string]struct{})
	for _, tenantID := range tenantIDSlice {
		tenantIDs[tenantID] = struct{}{}
	}

	for {
		if len(tenantIDs) == 0 {
			break
		}
		// retrieve request
		reqW := <-wc.reqCh
		wc.errCh <- nil

		req := reqW.req

		tenantID := mcontext.TenantIDFromContext(reqW.ctx)
		if _, ok := tenantIDs[tenantID]; !ok {
			t.Fatalf("tenant ID %s unexpected", tenantID)
		}
		delete(tenantIDs, tenantID)

		verify(t, req)
	}
}

func TestWriter_Run(t *testing.T) {

	for _, tc := range []runTestCase{
		{
			name: "send after reaching batchsize",
			send: func(ch chan pulsar.ReceivedSample) {
				// send two samples without channel ID
				ch <- newSampleNormal()
				ch <- newSampleInf()
			},
			verify: func(t testing.TB, wc *fakeWriteClient) {
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
			},
		},
		{
			name: "send after reaching batchsize with tenants",
			send: func(ch chan pulsar.ReceivedSample) {
				// send two samples per tenant ID
				ch <- newSampleNormal()
				ch <- withTenantID(newSampleNormal(), "team-a")
				ch <- withTenantID(newSampleNormal(), "team-b")
				ch <- newSampleInf()
				ch <- withTenantID(newSampleInf(), "team-a")
				ch <- withTenantID(newSampleInf(), "team-b")
			},
			verify: func(t testing.TB, wc *fakeWriteClient) {
				testForTenantIDs(t, wc, []string{"", "team-a", "team-b"}, func(t testing.TB, req *prompb.WriteRequest) {
					require.Equal(t, 2, len(req.Timeseries))

					// first sample
					assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
					assert.Equal(t, []prompb.Sample{sampleNormal}, req.Timeseries[0].Samples)

					// second sample
					assert.Equal(t, labelsPrompb, req.Timeseries[1].Labels)
					assert.Equal(t, []prompb.Sample{sampleInf}, req.Timeseries[1].Samples)
				})
			},
		},
		{
			name: "send after reaching max delay",
			send: func(ch chan pulsar.ReceivedSample) {
				// send a sample
				ch <- newSampleNormal()
			},
			verify: func(t testing.TB, wc *fakeWriteClient) {
				reqW := <-wc.reqCh
				wc.errCh <- nil

				req := reqW.req
				require.Equal(t, 1, len(req.Timeseries))

				// first sample
				assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
				assert.Equal(t, []prompb.Sample{sampleNormal}, req.Timeseries[0].Samples)
			},
			batchMaxDelay: time.Microsecond,
		},
		{
			name: "send after reaching max delay with tenants",
			send: func(ch chan pulsar.ReceivedSample) {
				// send sample per tenant ID
				ch <- newSampleNormal()
				ch <- withTenantID(newSampleNormal(), "team-a")
				ch <- withTenantID(newSampleNormal(), "team-b")
			},
			verify: func(t testing.TB, wc *fakeWriteClient) {
				testForTenantIDs(t, wc, []string{"", "team-a", "team-b"}, func(t testing.TB, req *prompb.WriteRequest) {
					require.Equal(t, 1, len(req.Timeseries))

					// first sample
					assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
					assert.Equal(t, []prompb.Sample{sampleNormal}, req.Timeseries[0].Samples)
				})
			},
			batchMaxDelay: time.Microsecond,
		},
		{
			name: "send after recoverable error",
			send: func(ch chan pulsar.ReceivedSample) {
				// send sample
				ch <- newSampleNormal()
				ch <- newSampleInf()
			},
			verify: func(t testing.TB, wc *fakeWriteClient) {
				// retrieve request but error
				<-wc.reqCh
				wc.errCh <- fmt.Errorf("I am very recoverable: %w", remote.RecoverableError{})

				// receive the second time round
				reqW := <-wc.reqCh
				wc.errCh <- nil

				req := reqW.req
				require.Equal(t, 1, len(req.Timeseries))

				// first sample
				assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
				assert.Equal(t, []prompb.Sample{sampleNormal}, req.Timeseries[0].Samples)

				reqW = <-wc.reqCh
				wc.errCh <- nil

				req = reqW.req
				require.Equal(t, 1, len(req.Timeseries))

				// second sample
				assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
				assert.Equal(t, []prompb.Sample{sampleInf}, req.Timeseries[0].Samples)
			},
			batchSize: 1,
		},
		{
			name: "send after unrecoverable error",
			send: func(ch chan pulsar.ReceivedSample) {
				// send sample
				ch <- newSampleNormal()
				ch <- newSampleInf()
			},
			verify: func(t testing.TB, wc *fakeWriteClient) {
				// retrieve request but error
				<-wc.reqCh
				wc.errCh <- errors.New("I am very unrecoverable")

				// receive the second time round
				reqW := <-wc.reqCh
				wc.errCh <- nil

				req := reqW.req
				require.Equal(t, 1, len(req.Timeseries))

				// second sample only
				assert.Equal(t, labelsPrompb, req.Timeseries[0].Labels)
				assert.Equal(t, []prompb.Sample{sampleInf}, req.Timeseries[0].Samples)

			},
			batchSize: 1,
		},
	} {
		t.Run(tc.name, tc.test)
	}
}

func benchmarkWriter(tenants, batchSize int, b *testing.B) {
	samplesCh := make(chan pulsar.ReceivedSample)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWrite()
	w.BatchSize = batchSize

	wc := newFakeWriteClient(b)
	wc.benchmark = true

	// watch channel
	go func() {
		err := w.Run(ctx, samplesCh, wc)
		require.NoError(b, err)
	}()

	tenantNames := make([]string, tenants)
	for t := 0; t < tenants; t++ {
		tenantNames[t] = fmt.Sprintf("team-%d", t)
	}

	b.StartTimer()
	for n := 0; n < b.N; n++ {
		for t := 0; t < tenants; t++ {
			samplesCh <- withTenantID(newSampleNormal(), tenantNames[t])
		}
	}
	b.ReportMetric(float64(tenants)*float64(b.N), "samples")
	b.StopTimer()
}

func BenchmarkWriter1Tenants100BatchSize(b *testing.B) {
	benchmarkWriter(1, 100, b)
}
func BenchmarkWriter50Tenants100BatchSize(b *testing.B) {
	benchmarkWriter(50, 100, b)
}
func BenchmarkWriter100Tenants100BatchSize(b *testing.B) {
	benchmarkWriter(100, 1000, b)
}
func BenchmarkWriter500Tenants100BatchSize(b *testing.B) {
	benchmarkWriter(500, 100, b)
}
