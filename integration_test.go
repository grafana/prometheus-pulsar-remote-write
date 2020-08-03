package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	promconfig "github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type Clocker interface {
	Now() time.Time
}

var clock Clocker = &testClock{}

type testClock struct {
	t time.Time
}

// every call to now will add another second to the time
func (c *testClock) Now() time.Time {
	if c.t.IsZero() {
		c.t = time.Unix(1588462000, 0)
	} else {
		c.t = c.t.Add(time.Second)
	}
	return c.t
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

// return a random string
func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

const envTestPulsarURL = "TEST_PULSAR_URL"

func skipWithoutPulsar(t *testing.T) {
	if os.Getenv(envTestPulsarURL) == "" {
		t.Skipf("Integration tests skipped as not pulsar URL provided in environment variable %s.", envTestPulsarURL)
	}
}

func runBatch(writeClient *remote.Client, from, to int) error {
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

func getRandomFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type testIntegration struct {
	remoteWriteConfigHook func(*remote.ClientConfig)
	consumeMessage        func(pulsar.Message)
}

func (ti *testIntegration) test(t *testing.T) {
	skipWithoutPulsar(t)

	port, err := getRandomFreePort()
	assert.Nil(t, err)
	host := fmt.Sprintf("127.0.0.1:%d", port)

	pulsarTopic := fmt.Sprintf("metrics-test-%s", randSeq(8))
	pulsarURL := os.Getenv(envTestPulsarURL)

	os.Args = []string{
		"go test",
		// set pulsar url from environment
		fmt.Sprintf(`--pulsar.url=%s`, pulsarURL),
		fmt.Sprintf(`--pulsar.topic=%s`, pulsarTopic),
		// use a random port to listen
		fmt.Sprintf(`--web.listen-address=%s`, host),
	}
	cfg := parseFlags()

	logger := log.NewNopLogger()

	writers, readers := buildClients(logger, cfg)

	server := &http.Server{
		Addr: cfg.listenAddr,
	}
	defer server.Close()

	go func() {
		err := serve(logger, cfg, server, writers, readers)
		if err == http.ErrServerClosed {
			return
		}
		assert.Nil(t, err)
	}()

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

	remoteWrite, err := remote.NewClient("test", remoteWriteConfig)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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

func TestIntegrationDefaultJSON(t *testing.T) {
	ti := &testIntegration{}
	ti.test(t)
}

func TestIntegrationBasicAuthTenantIDJSON(t *testing.T) {
	ti := &testIntegration{
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

func TestMain(m *testing.M) {
	flag.Parse()
	// reduce verbosity of logrus which is used by the pulsar golang library
	if !testing.Verbose() {
		logrus.SetLevel(logrus.WarnLevel)
	}
	os.Exit(m.Run())
}
