package pulsar

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/pkg/context"
)

// Client allows sending batches of Prometheus samples to InfluxDB.
type Client struct {
	logger log.Logger

	client        pulsar.Client
	serializer    Serializer
	replicaLabels []model.LabelName
	producer      pulsar.Producer
}

type ClientOptions pulsar.ClientOptions

type Authentication pulsar.Authentication

func NewAuthenticationTLS(certificatePath string, privateKeyPath string) Authentication {
	return pulsar.NewAuthenticationTLS(certificatePath, privateKeyPath)
}

type Config struct {
	ClientOptions
	Topic         string
	Logger        log.Logger
	ReplicaLabels []string
}

func NewClient(config Config) (*Client, error) {
	logger := config.Logger
	if logger == nil {
		logger = log.NewNopLogger()
	}

	replicaLabels := make([]model.LabelName, len(config.ReplicaLabels))
	for pos := range config.ReplicaLabels {
		replicaLabels[pos] = model.LabelName(config.ReplicaLabels[pos])
	}

	c, err := pulsar.NewClient(pulsar.ClientOptions(config.ClientOptions))
	if err != nil {
		return nil, err
	}
	producer, err := c.CreateProducer(pulsar.ProducerOptions{
		Topic: config.Topic,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating producer: %w", err)
	}
	return &Client{
		logger: logger,

		client:        c,
		producer:      producer,
		serializer:    NewJSONSerializer(),
		replicaLabels: replicaLabels,
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

func (c *Client) Close() error {
	if c.producer != nil {
		err := c.producer.Flush()
		if err != nil {
			return err
		}
		c.producer.Close()
		c.producer = nil
	}
	c.client.Close()
	return nil
}

func (c *Client) Write(ctx context.Context, samples model.Samples) error {
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
		c.producer.SendAsync(
			context.Background(),
			&pulsar.ProducerMessage{
				Payload: bytes,
				Key:     s.partitionKey(c.replicaLabels),
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
	return c.producer.Flush()
}
