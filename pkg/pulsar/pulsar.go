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
	topic         string
	subscription  string

	_producerLock sync.Mutex
	_producer     pulsar.Producer
	_consumerLock sync.Mutex
	_consumer     pulsar.Consumer
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
	Subscription  string
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

	return &Client{
		logger: logger,

		client:        c,
		serializer:    NewJSONSerializer(),
		replicaLabels: replicaLabels,
		topic:         config.Topic,
		subscription:  config.Subscription,
	}, nil
}

func (c *Client) InitProducer() error {
	_, err := c.producer()
	return err
}

func (c *Client) producer() (pulsar.Producer, error) {
	c._producerLock.Lock()
	defer c._producerLock.Unlock()
	if c._producer != nil {
		return c._producer, nil
	}

	producer, err := c.client.CreateProducer(pulsar.ProducerOptions{
		Topic: c.topic,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating producer: %w", err)
	}
	c._producer = producer

	return producer, nil
}

func (c *Client) InitConsumer() error {
	_, err := c.consumer()
	return err
}

func (c *Client) consumer() (pulsar.Consumer, error) {
	c._consumerLock.Lock()
	defer c._consumerLock.Unlock()
	if c._consumer != nil {
		return c._consumer, nil
	}

	consumer, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic:            c.topic,
		SubscriptionName: c.subscription,
		Type:             pulsar.KeyShared,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating consumer: %w", err)
	}
	c._consumer = consumer

	return consumer, nil
}

// Name identifies the client as a Pulsar client.
func (*Client) Name() string {
	return "pulsar"
}

func (c *Client) WithSerializer(s Serializer) *Client {
	c.serializer = s
	return c
}

func (c *Client) Close() error {
	c._producerLock.Lock()
	defer c._producerLock.Unlock()
	if c._producer != nil {
		err := c._producer.Flush()
		if err != nil {
			return err
		}
		c._producer.Close()
		c._producer = nil
	}

	c._consumerLock.Lock()
	defer c._consumerLock.Unlock()
	if c._consumer != nil {
		err := c._consumer.Unsubscribe()
		if err != nil {
			return err
		}
		c._consumer.Close()
		c._consumer = nil
	}

	c.client.Close()
	return nil
}

type ReceivedSample struct {
	Sample  *model.Sample
	Context context.Context
	Ack     func()
	Nack    func()
}

// Receiver watches the queue for relevant samples, unserializes them and sends
// via the sampleCh. A channel is returned itself, to wait for the work loop to
// finish
func (c *Client) Receiver(ctx context.Context, sampleCh chan ReceivedSample) (done chan struct{}, err error) {
	consumer, err := c.consumer()
	if err != nil {
		return nil, err
	}

	done = make(chan struct{})

	go func() {
		defer close(done)

		recv := consumer.Chan()

		for {
			select {
			case msg := <-recv:

				id := msg.Message.ID()
				payload := msg.Message.Payload()

				sample, err := c.serializer.Unmarshal(payload)
				if err != nil {
					// TODO: Decide if that should be ack or nack
					_ = level.Warn(c.logger).Log(
						"msg", "Cannot unserialize payload, skipping message",
						"err", err,
						"msg_id", id,
						"payload", string(payload),
					)
					continue
				}

				sampleCh <- ReceivedSample{
					Context: mcontext.ContextWithTenantID(ctx, sample.TenantID),
					Nack: func() {
						consumer.NackID(id)
					},
					Ack: func() {
						consumer.AckID(id)
					},
					Sample: &model.Sample{
						Metric:    sample.Metric,
						Timestamp: sample.Value.Timestamp,
						Value:     sample.Value.Value,
					},
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return done, nil
}

func (c *Client) Write(ctx context.Context, samples model.Samples) error {
	producer, err := c.producer()
	if err != nil {
		return err
	}

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
	return producer.Flush()
}
