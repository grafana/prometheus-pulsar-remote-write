package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	promconfig "github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/stretchr/testify/assert"
)

func runBatch(writeClient remote.WriteClient, from, to int) error {
	var (
		req = prompb.WriteRequest{
			Timeseries: make([]prompb.TimeSeries, 0, to-from),
		}
		now = clock.Now().UnixNano() / int64(time.Millisecond)
	)

	for i := from; i < to; i++ {
		timeseries := prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "node_cpu_seconds_total"},
				{Name: "job", Value: "node_exporter"},
				{Name: "instance", Value: fmt.Sprintf("instance%000d", i)},
				{Name: "cpu", Value: "0"},
				{Name: "mode", Value: "idle"},
			},
			Samples: []prompb.Sample{{
				Timestamp: now,
				Value:     rand.Float64(),
			}},
		}
		req.Timeseries = append(req.Timeseries, timeseries)
	}

	data, err := proto.Marshal(&req)
	if err != nil {
		return err
	}

	compressed := snappy.Encode(nil, data)

	if err := writeClient.Store(context.Background(), compressed); err != nil {
		return err
	}

	return nil
}

type testProduceIntegration struct {
	remoteWriteConfigHook func(*remote.ClientConfig)
	consumeMessage        func(pulsar.Message)
}

func isReady(ctx context.Context, url string) error {
	var err error
	for {
		select {
		case <-time.NewTimer(50 * time.Millisecond).C:
			req, rErr := http.NewRequest("GET", url, nil)
			if rErr != nil {
				err = rErr
				continue
			}

			resp, rErr := http.DefaultClient.Do(req.WithContext(ctx))
			//defer resp.Body.Close()
			if rErr != nil {
				err = rErr
				continue
			}

			if resp.StatusCode == 200 {
				return nil
			}
			err = fmt.Errorf("unexpected status code %d", resp.StatusCode)
		case <-ctx.Done():
			return fmt.Errorf("context timeout waiting for readiness %s: %s", url, err)
		}
	}
}

func (ti *testProduceIntegration) test(t *testing.T) {
	skipWithoutPulsar(t)

	port, err := getRandomFreePort()
	assert.Nil(t, err)
	host := fmt.Sprintf("127.0.0.1:%d", port)

	pulsarTopic := fmt.Sprintf("metrics-test-%s", randSeq(8))
	pulsarURL := os.Getenv(envTestPulsarURL)

	args := []string{
		"produce",
		// set pulsar url from environment
		fmt.Sprintf(`--pulsar.url=%s`, pulsarURL),
		fmt.Sprintf(`--pulsar.topic=%s`, pulsarTopic),
		// use a random port to listen
		fmt.Sprintf(`--web.listen-address=%s`, host),
	}

	adapterCtx, adapterCancel := context.WithCancel(context.Background())
	defer adapterCancel()

	go func() {
		t.Logf("run adapter with args=%v", args)
		err := app.Run(adapterCtx, args...)
		assert.Nil(t, err)
	}()

	// wait till server is ready
	readyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := isReady(readyCtx, fmt.Sprintf("http://%s/ready", host)); err != nil {
		t.Fatalf("remote adapter is not ready: %v", err)
	}

	// build a remote writer
	remoteWriteURL, err := url.Parse(fmt.Sprintf("http://%s/write", host))
	assert.Nil(t, err)

	remoteWriteConfig := &remote.ClientConfig{
		URL: &promconfig.URL{
			URL: remoteWriteURL,
		},
		Timeout: prommodel.Duration(time.Second),
	}

	if f := ti.remoteWriteConfigHook; f != nil {
		f(remoteWriteConfig)
	}

	remoteWrite, err := remote.NewWriteClient("test", remoteWriteConfig)
	assert.Nil(t, err)

	// send 2 times a batch of 4 messages to the bus
	batchSize := 4
	batches := 2
	for i := 0; i < batches; i++ {
		err = runBatch(remoteWrite, 0, batchSize)
		assert.Nil(t, err)
	}

	pulsarClient, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: pulsarURL,
	})
	assert.Nil(t, err)
	defer pulsarClient.Close()

	consumer, err := pulsarClient.Subscribe(pulsar.ConsumerOptions{
		Topic:                       pulsarTopic,
		SubscriptionName:            "go-test",
		Type:                        pulsar.Shared,
		SubscriptionInitialPosition: pulsar.SubscriptionPositionEarliest,
	})
	assert.Nil(t, err)
	defer consumer.Close()

	// set deadline if nothing can be consumed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// receive messages on topic
	for i := 0; i < batchSize*batches; i++ {
		msg, err := consumer.Receive(ctx)
		assert.Nil(t, err)
		if err != nil {
			break
		}

		// TODO: We should verify the order of the messages entries here (specifically timestamp order)

		t.Logf("received message body=%s", msg.Payload())
		if f := ti.consumeMessage; f != nil {
			f(msg)
		}

		consumer.Ack(msg)
	}

	err = consumer.Unsubscribe()
	assert.Nil(t, err)
}

func TestIntegrationProduceDefaultJSON(t *testing.T) {
	ti := &testProduceIntegration{}
	ti.test(t)
}

func TestIntegrationProduceBasicAuthTenantIDJSON(t *testing.T) {
	ti := &testProduceIntegration{
		remoteWriteConfigHook: func(c *remote.ClientConfig) {
			c.HTTPClientConfig.BasicAuth = &promconfig.BasicAuth{
				Username: "my-org-id",
				Password: "token",
			}
		},
		consumeMessage: func(msg pulsar.Message) {
			data := struct {
				TenantID string `json:"tenant_id"`
			}{}
			err := json.Unmarshal(msg.Payload(), &data)
			assert.Nil(t, err)
			assert.Equal(t, "my-org-id", data.TenantID)
		},
	}
	ti.test(t)
}
