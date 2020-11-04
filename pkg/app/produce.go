package app

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/pkg/context"
	"github.com/grafana/prometheus-pulsar-remote-write/pkg/pulsar"
)

const errSendingSamples = "Error sending samples to remote storage"

type produceCommand struct {
	app  *App
	name string

	writePath     string
	replicaLabels []string
	metrics       *metrics
	sendTimeout   time.Duration
	pulsar        *pulsarConfig
}

func newProduceCommand(app *App) *produceCommand {
	name := "produce"
	p := &produceCommand{
		name:   name,
		app:    app,
		pulsar: &pulsarConfig{},
	}

	cmd := app.app.Command(name, "Receive remote_write requests and produce messages on the pulsar bus").Default()

	p.pulsar.addFlags(cmd)
	cmd.Flag("send-timeout", "The timeout to use when sending samples to the remote storage.").
		Default("30s").DurationVar(&p.sendTimeout)
	cmd.Flag("web.write-path", "Path under which to receive remote_write requests.").
		Default("/write").StringVar(&p.writePath)
	cmd.Flag("replica-label", "External label to identify replicas. Can be specified multiple times.").
		Default("__replica__").StringsVar(&p.replicaLabels)

	return p
}

type writer interface {
	Write(ctx context.Context, samples model.Samples) error
	Name() string
	Close() error
}

func (p *produceCommand) pulsarClient() (*pulsar.Client, error) {
	clientOptions, err := p.pulsar.clientOptions()
	if err != nil {
		return nil, err
	}

	// set write timeout
	clientOptions.OperationTimeout = p.sendTimeout

	client, err := p.pulsar.client(
		p.app.logger,
		pulsarClientWithClientOpts(clientOptions),
		pulsarClientWithReplicaLabels(p.replicaLabels),
	)
	if err != nil {
		return nil, err
	}

	return client, client.InitProducer()
}

func (p *produceCommand) buildWriters() ([]writer, error) {
	var writers []writer
	if p.pulsar.url != "" {

		// create client
		c, err := p.pulsarClient()
		if err != nil {
			return nil, err
		}
		_ = level.Info(p.app.logger).Log("msg", "Created writer successfully", "name", c.Name())

		writers = append(writers, c)
	}
	return writers, nil
}

func (p *produceCommand) closeWriters(writers []writer) {
	for _, w := range writers {
		if err := w.Close(); err != nil {
			_ = level.Warn(p.app.logger).Log("msg", "Failed to close writer", "name", w.Name(), "err", err)
		}
	}
}

func (p *produceCommand) run(ctx context.Context) error {
	if p.metrics == nil {
		p.metrics = newMetrics(p.app.registry)
	}
	ctx, finish := p.app.signalHandler(ctx)
	defer finish()

	_ = level.Info(p.app.logger).Log("msg", "Starting in produce mode")

	srv := p.app.newServer()

	writers, err := p.buildWriters()
	if err != nil {
		return err
	}
	defer p.closeWriters(writers)

	middleware := func(next http.HandlerFunc) http.Handler {
		return mcontext.TenantIDHandler(next)
	}

	srv.mux.Handle(p.writePath, middleware(func(w http.ResponseWriter, r *http.Request) {
		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			_ = level.Error(p.app.logger).Log("msg", "Read error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			_ = level.Error(p.app.logger).Log("msg", "Decode error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			_ = level.Error(p.app.logger).Log("msg", "Unmarshal error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		samples := protoToSamples(&req)
		p.metrics.receivedSamples.Add(float64(len(samples)))

		// error if no writer is configured
		if len(writers) == 0 {
			http.Error(w, "No write destinations configured", http.StatusBadGateway)
			return
		}

		var wg sync.WaitGroup
		var errs []error = make([]error, len(writers))
		for pos, w := range writers {
			wg.Add(1)
			go func(ctx context.Context, pos int, rw writer) {
				errs[pos] = p.sendSamples(rw, ctx, samples)
				wg.Done()
			}(r.Context(), pos, w)
		}
		wg.Wait()

		var failedWriters []string
		for pos, w := range writers {
			if errs[pos] != nil {
				failedWriters = append(failedWriters, w.Name())
			}
		}

		if len(failedWriters) > 0 {
			http.Error(
				w,
				fmt.Sprintf("%ss: %s", errSendingSamples, strings.Join(failedWriters, ", ")),
				http.StatusInternalServerError,
			)
			return
		}

	}))

	return srv.run(ctx)
}

func protoToSamples(req *prompb.WriteRequest) model.Samples {
	var samples model.Samples
	for _, ts := range req.Timeseries {
		metric := make(model.Metric, len(ts.Labels))
		for _, l := range ts.Labels {
			metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
		}

		for _, s := range ts.Samples {
			samples = append(samples, &model.Sample{
				Metric:    metric,
				Value:     model.SampleValue(s.Value),
				Timestamp: model.Time(s.Timestamp),
			})
		}
	}
	return samples
}

func (p *produceCommand) sendSamples(w writer, ctx context.Context, samples model.Samples) error {
	begin := time.Now()
	err := w.Write(ctx, samples)
	duration := time.Since(begin).Seconds()
	p.metrics.sentSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	p.metrics.sentBatchDuration.WithLabelValues(w.Name()).Observe(duration)
	if err != nil {
		_ = level.Warn(p.app.logger).Log("msg", errSendingSamples, "err", err, "storage", w.Name(), "num_samples", len(samples))
		p.metrics.failedSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
		return err
	}
	return nil
}
