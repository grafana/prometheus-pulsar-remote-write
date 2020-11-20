package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/go-kit/kit/log/level"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/grafana/prometheus-pulsar-remote-write/pkg/pulsar"
	"github.com/grafana/prometheus-pulsar-remote-write/pkg/remote"
)

type consumeCommand struct {
	app  *App
	name string

	sendTimeout        time.Duration
	pulsar             *pulsarConfig
	pulsarSubscription string
	remoteWriteURL     string
}

func newConsumeCommand(app *App) *consumeCommand {
	name := "consume"
	c := &consumeCommand{
		name:   name,
		app:    app,
		pulsar: &pulsarConfig{},
	}

	cmd := app.app.Command(name, "Consume metrics on the pulsar bus and send them as remote_write requests")

	c.pulsar.addFlags(cmd)
	cmd.Flag("send-timeout", "The timeout to use when sending samples to the remote_write endpoint.").
		Default("30s").DurationVar(&c.sendTimeout)
	cmd.Flag("pulsar.subscription", "The subscription name used to consume messages of the bus. It is important that if you are reading with multiple consumers, all of them need to share the same subscription name.").
		Default("pulsar-adapter").StringVar(&c.pulsarSubscription)
	cmd.Flag("remote-write.url", "The URL of remote_write endpoint to forward the metrics to.").Required().
		StringVar(&c.remoteWriteURL)

	return c
}

func (c *consumeCommand) pulsarClient() (*pulsar.Client, error) {
	// TODO: Not too sure how relevant it is for consuming from the bus
	//clientOptions.OperationTimeout = p.readTimeout

	client, err := c.pulsar.client(
		c.app.logger,
		pulsarClientWithSubscription(c.pulsarSubscription),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (c *consumeCommand) run(ctx context.Context) error {
	ctx, finish := c.app.signalHandler(ctx)
	defer finish()

	if c.pulsar.url == "" {
		return errors.New("no pulsar URL defined")
	}

	client, err := c.pulsarClient()
	if err != nil {
		return fmt.Errorf("failed creating consumer: %w", err)
	}
	_ = level.Info(c.app.logger).Log("msg", "Created consumer successfully", "name", client.Name())

	remoteURL, err := url.Parse(c.remoteWriteURL)
	if err != nil {
		return fmt.Errorf("failed creating remote_write URL: %w", err)
	}

	// create remote write client
	remoteClient, err := remote.NewWriteClient(&remote.ClientConfig{
		URL:     &config_util.URL{URL: remoteURL},
		Timeout: model.Duration(c.sendTimeout),
	})
	if err != nil {
		return fmt.Errorf("failed creating remote_write client: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	// start server to expose metrics/pprof
	srv := c.app.newServer()
	go func() {
		if err := srv.run(ctx); err != nil {
			_ = level.Error(c.app.logger).Log("msg", "Error running web server", "error", err)
			cancel()
		}
	}()

	sampleCh := make(chan pulsar.ReceivedSample)

	done, err := client.Receiver(ctx, sampleCh)
	if err != nil {
		return err
	}

	go func() {
		<-done
		_ = level.Debug(c.app.logger).Log("msg", "Recevier stopped", "name", client.Name())
		cancel()
	}()

	write := remote.NewWrite(remote.WithLogger(c.app.logger))

	return write.Run(ctx, sampleCh, remoteClient)
}
