package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/pkg/context"
	mpulsar "github.com/grafana/prometheus-pulsar-remote-write/pkg/pulsar"
)

type testConsumeIntegration struct {
	tenantID string
}

type reqWithContext struct {
	req *prompb.WriteRequest
	ctx context.Context
}

type remoteWrite struct {
	*httptest.Server

	reqCh chan reqWithContext
}

func newRemoteWrite(t testing.TB) *remoteWrite {
	rw := &remoteWrite{
		reqCh: make(chan reqWithContext),
	}
	rw.Server = httptest.NewServer(mcontext.TenantIDHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/push", r.URL.Path)
		assert.Equal(t, "snappy", r.Header.Get("Content-Encoding"))

		compressed, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)

		reqBuf, err := snappy.Decode(nil, compressed)
		require.NoError(t, err)

		var req prompb.WriteRequest
		require.NoError(t, proto.Unmarshal(reqBuf, &req))

		rw.reqCh <- reqWithContext{
			req: &req,
			ctx: r.Context(),
		}
		fmt.Fprintf(w, "ok")

	})))

	return rw
}

func instanceLabels(from, to int) []string {
	labels := make([]string, to-from)
	for i := from; i < to; i++ {
		labels[i-from] = fmt.Sprintf("instance%000d", i)
	}
	return labels
}

func produceBatch(t testing.TB, ctx context.Context, producer pulsar.Producer, tenantID string, serializer mpulsar.Serializer, instances []string) {
	var (
		now = clock.Now()
	)

	var sample = mpulsar.Sample{
		TenantID: tenantID,
		Metric: model.Metric{
			"__name__": "node_cpu_seconds_total",
			"job":      "node_exporter",
			"cpu":      "0",
			"mode":     "idle",
		},
		Value: model.SamplePair{
			Timestamp: model.TimeFromUnixNano(now.UnixNano()),
		},
	}

	var wg sync.WaitGroup

	for _, instance := range instances {
		sample.Metric[model.LabelName("instance")] = model.LabelValue(instance)
		sample.Value.Value = model.SampleValue(rand.Float64())

		bytes, err := serializer.Marshal(&sample)
		require.NoError(t, err)

		wg.Add(1)
		producer.SendAsync(
			ctx,
			&pulsar.ProducerMessage{
				Payload: bytes,
				Key:     "fake",
			},
			func(id pulsar.MessageID, message *pulsar.ProducerMessage, err error) {
				defer wg.Done()
				require.NoError(t, err, "failed")
			},
		)
	}

	wg.Wait()
	require.NoError(t, producer.Flush())
}

func (ti *testConsumeIntegration) test(t *testing.T) {
	skipWithoutPulsar(t)

	metricPort, err := getRandomFreePort()
	assert.Nil(t, err)
	metricHost := fmt.Sprintf("127.0.0.1:%d", metricPort)

	rw := newRemoteWrite(t)
	defer rw.Close()

	pulsarTopic := fmt.Sprintf("metrics-test-%s", randSeq(8))
	pulsarURL := os.Getenv(envTestPulsarURL)

	args := []string{
		"consume",
		// set pulsar url from environment
		fmt.Sprintf(`--pulsar.url=%s`, pulsarURL),
		fmt.Sprintf(`--pulsar.topic=%s`, pulsarTopic),
		// use a random subscription name
		fmt.Sprintf(`--pulsar.subscription=%s`, pulsarTopic),
		// use a random port to listen for metrics
		fmt.Sprintf(`--web.listen-address=%s`, metricHost),
		// specifiy the remote write endpoint
		fmt.Sprintf(`--remote-write.url=%s/api/v1/push`, rw.URL),
	}

	adapterCtx, adapterCancel := context.WithCancel(context.Background())
	defer adapterCancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Logf("run adapter with args=%v", args)
		err := app.Run(adapterCtx, args...)
		assert.NoError(t, err)
	}()

	// connect to pulsar to produce some messages
	pulsarClient, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: pulsarURL,
	})
	require.NoError(t, err)
	defer pulsarClient.Close()

	producer, err := pulsarClient.CreateProducer(pulsar.ProducerOptions{
		Topic: pulsarTopic,
	})
	require.NoError(t, err)
	defer producer.Close()

	instanceLabels := instanceLabels(1, 501)
	samplesEach := 4

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for i := samplesEach; i > 0; i-- {
			produceBatch(t, ctx, producer, ti.tenantID, mpulsar.NewJSONSerializer(), instanceLabels)
		}

		// send an extra batch we don't check for, as for some reason sometimes
		// a flake occurs where pulsar won't forward messages from the last
		// batch. (according to pulsar's metrics, they have arrived correctly,
		// but are not sent out to the subscription)
		produceBatch(t, ctx, producer, ti.tenantID, mpulsar.NewJSONSerializer(), instanceLabels)

		producer.Close()
	}()

	// receive all remote written samples and verify all samples per instance have arrived
	var instancesToBeReceivedByTimestamp = make(instanceSampleMap)

	for {
		r := <-rw.reqCh
		t.Logf("received request with %d series", len(r.req.Timeseries))

		assert.Equal(t, ti.tenantID, mcontext.TenantIDFromContext(r.ctx), "expect tenantID to be propagated")

		for _, series := range r.req.Timeseries {
			instance, ok := labelGet(series.Labels, "instance")
			if !ok {
				panic("no instance label found")
			}

			for _, sample := range series.Samples {
				toBeReceived, ok := instancesToBeReceivedByTimestamp[sample.Timestamp]
				if !ok {
					toBeReceived = newInstanceMap(instanceLabels)
					instancesToBeReceivedByTimestamp[sample.Timestamp] = toBeReceived
				}
				delete(toBeReceived, instance)
			}
		}

		if instancesToBeReceivedByTimestamp.finishedSamples() == samplesEach {
			t.Logf("received %d samples per series", samplesEach)
			break
		}
	}

	adapterCancel()
	wg.Wait()
}

type instanceMap map[string]struct{}

func newInstanceMap(instances []string) instanceMap {
	var m = make(instanceMap, len(instances))
	for _, instance := range instances {
		m[instance] = struct{}{}
	}
	return m
}

type instanceSampleMap map[int64]instanceMap

func (m instanceSampleMap) finishedSamples() int {
	var finished int
	for _, v := range m {
		if len(v) == 0 {
			finished += 1
		}
	}
	return finished
}

func labelGet(labels []prompb.Label, name string) (string, bool) {
	for _, l := range labels {
		if l.Name == name {
			return l.Value, true
		}
	}
	return "", false
}

func TestIntegrationConsumeDefaultJSON(t *testing.T) {
	ti := &testConsumeIntegration{}
	ti.test(t)
}

func TestIntegrationConsumeDefaultJSONWithTenantID(t *testing.T) {
	ti := &testConsumeIntegration{
		tenantID: "my-org-id",
	}
	ti.test(t)
}
