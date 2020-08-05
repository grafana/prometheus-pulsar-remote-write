package pulsar

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/context"
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	logger log.Logger

	client     pulsar.Client
	topic      string
	serializer Serializer
}

type ClientOptions pulsar.ClientOptions
type Config struct {
	ClientOptions
	Topic  string
	Logger log.Logger
}

func NewClient(config Config) (*Client, error) {
	logger := config.Logger
	if logger == nil {
		logger = log.NewNopLogger()
	}

	c, err := pulsar.NewClient(pulsar.ClientOptions(config.ClientOptions))
	if err != nil {
		return nil, err
	}
	return &Client{
		logger: logger,

		client:     c,
		topic:      config.Topic,
		serializer: NewJSONSerializer(),
	}, nil
}

// Name identifies the client as a Pulsar client.
func (c Client) Name() string {
	return "pulsar"
}

func (c *Client) WithSerializer(s Serializer) *Client {
	c.serializer = s
	return c
}

func (c *Client) Write(ctx context.Context, samples model.Samples) error {
	producer, err := c.client.CreateProducer(pulsar.ProducerOptions{
		Topic: c.topic,
	})
	if err != nil {
		return fmt.Errorf("error creating producer: %w", err)
	}
	defer producer.Close()

	tenantID := mcontext.TenantIDFromContext(ctx)

	var wg sync.WaitGroup

	for _, sample := range samples {
		s := NewSample(sample)
		s.TenantID = tenantID
		bytes, err := c.serializer.Marshal(s)
		if err != nil {
			_ = level.Warn(c.logger).Log("msg", "Cannot serialize, skipping sample", "err", err, "sample", sample)
			continue
		}

		wg.Add(1)
		producer.SendAsync(
			context.Background(),
			&pulsar.ProducerMessage{
				Payload: bytes,
			},
			func(id pulsar.MessageID, message *pulsar.ProducerMessage, err error) {
				defer wg.Done()
				if err != nil {
					_ = level.Warn(c.logger).Log("msg", "SendAsync failed", "err", err, "message", message)
				}
			},
		)
	}

	wg.Wait()
	return producer.Flush()
}
