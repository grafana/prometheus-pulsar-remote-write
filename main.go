// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The main package for the prometheus-pulsar-remote-write adapter
package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/prometheus/prompb"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/context"
	"github.com/grafana/prometheus-pulsar-remote-write/pulsar"
)

var (
	pulsarSerializerJSON           = "json"
	pulsarSerializerJSONCompat     = "json-compat"
	pulsarSerializerAvroJSONCompat = "avro-json-compat"
)

const errSendingSamples = "Error sending samples to remote storage"

var pulsarSerializerHelp = fmt.Sprintf(`Specifies the serialization format

%s: JSON default format as defined by github.com/prometheus/common/model

{
  "metric": {
    "__name__": "foo",
    "labelfoo": "label-bar"
  },
  "value": [
    0,
    "456"
  ]
}

%s: JSON compat provides compatability with github.com/liangyuanpeng/prometheus-pulsar-adapter

{
  "name": "foo",
  "labels": {
    "__name__": "foo",
    "labelfoo": "label-bar"
  },
  "value": "456",
  "timestamp": "1970-01-01T00:00:00Z"
}

%s[=<path to schema>]: AVRO-JSON which can optionally read a custom schema

Default schema:
%s

`,
	pulsarSerializerJSON,
	pulsarSerializerJSONCompat,
	pulsarSerializerAvroJSONCompat,
	pulsar.AvroJSONDefaultSchema,
)

type config struct {
	pulsarURL            string
	pulsarSerializer     string
	pulsarTopic          string
	pulsarConnectTimeout time.Duration
	remoteTimeout        time.Duration
	listenAddr           string
	telemetryPath        string
	writePath            string
	promlogConfig        promlog.Config
}

var (
	receivedSamples = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "received_samples_total",
			Help: "Total number of received samples.",
		},
	)
	sentSamples = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sent_samples_total",
			Help: "Total number of processed samples sent to remote storage.",
		},
		[]string{"remote"},
	)
	failedSamples = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "failed_samples_total",
			Help: "Total number of processed samples which failed on send to remote storage.",
		},
		[]string{"remote"},
	)
	sentBatchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sent_batch_duration_seconds",
			Help:    "Duration of sample batch send calls to the remote storage.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"remote"},
	)
)

func main() {
	cfg := parseFlags()

	logger := promlog.New(&cfg.promlogConfig)

	// reduce verbosity of logrus which is used by the pulsar golang library
	if cfg.promlogConfig.Level.String() != "debug" {
		logrus.SetLevel(logrus.WarnLevel)
	}

	writers, readers := buildClients(logger, cfg)
	server := &http.Server{
		Addr: cfg.listenAddr,
	}
	if err := serve(logger, cfg, server, writers, readers); err != nil {
		_ = level.Error(logger).Log("msg", "Failed to listen", "addr", cfg.listenAddr, "err", err)
		os.Exit(1)
	}
}

func parseFlags() *config {
	a := kingpin.New(filepath.Base(os.Args[0]), "Pulsar Remote storage adapter for Prometheus")
	a.HelpFlag.Short('h')

	cfg := &config{
		//pulsardbPassword: os.Getenv("INFLUXDB_PW"),
		promlogConfig: promlog.Config{},
	}

	a.Flag("send-timeout", "The timeout to use when sending samples to the remote storage.").
		Default("30s").DurationVar(&cfg.remoteTimeout)
	a.Flag("web.listen-address", "Address to listen on for web endpoints.").
		Default(":9201").StringVar(&cfg.listenAddr)
	a.Flag("web.telemetry-path", "Path under which to expose metrics.").
		Default("/metrics").StringVar(&cfg.telemetryPath)
	a.Flag("web.write-path", "Path under which to receive remote_write requests.").
		Default("/write").StringVar(&cfg.writePath)
	a.Flag("pulsar.url", "The URL of the remote Pulsar server to send samples to. Example: pulsar://pulsar-proxy:6650. None, if empty.").
		Default("").StringVar(&cfg.pulsarURL)
	a.Flag("pulsar.connection-timeout", "The timeout to use when connection to the remote Pulsar server.").
		Default("30s").DurationVar(&cfg.pulsarConnectTimeout)
	a.Flag("pulsar.serializer", pulsarSerializerHelp).
		Default("json").StringVar(&cfg.pulsarSerializer)
	a.Flag("pulsar.topic", "The Pulsar topic to publish the metrics on").
		Default("metrics").StringVar(&cfg.pulsarTopic)

	flag.AddFlags(a, &cfg.promlogConfig)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	return cfg
}

type writer interface {
	Write(ctx context.Context, samples model.Samples) error
	Name() string
}

type reader interface {
	Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error)
	Name() string
}

func buildClients(logger log.Logger, cfg *config) ([]writer, []reader) {
	var writers []writer
	var readers []reader
	if cfg.pulsarURL != "" {
		c, err := pulsar.NewClient(pulsar.Config{
			ClientOptions: pulsar.ClientOptions{
				URL:               cfg.pulsarURL,
				ConnectionTimeout: cfg.pulsarConnectTimeout,
				OperationTimeout:  cfg.remoteTimeout,
			},
			Topic:  cfg.pulsarTopic,
			Logger: log.With(logger, "storage", "Pulsar"),
		})
		if err != nil {
			_ = level.Error(logger).Log("msg", "Failed to initialize Pulsar", "err", err)
			os.Exit(1)
		}

		// check for the serializer settings
		switch cfg.pulsarSerializer {
		case pulsarSerializerJSON:
			c.WithSerializer(pulsar.NewJSONSerializer())
		case pulsarSerializerJSONCompat:
			c.WithSerializer(pulsar.NewJSONCompatSerializer())
		case pulsarSerializerAvroJSONCompat:
			serializer, err := pulsar.NewAvroJSONSerializer(nil)
			if err != nil {
				_ = level.Error(logger).Log("msg", "Failed to initialize Pulsar serializer", "err", err, "pulsar.serializer", cfg.pulsarSerializer)
				os.Exit(1)
			}
			c.WithSerializer(serializer)
		default:
			prefix := fmt.Sprintf("%s=", pulsarSerializerAvroJSONCompat)
			if strings.HasPrefix(cfg.pulsarSerializer, prefix) {
				filePath := cfg.pulsarSerializer[len(prefix):]
				file, err := os.Open(filePath)
				if err != nil {
					_ = level.Error(logger).Log("msg", "Failed to open Pulsar avro serializer schema", "err", err, "filepath", filePath)
					os.Exit(1)
				}
				defer file.Close()

				serializer, err := pulsar.NewAvroJSONSerializer(file)
				if err != nil {
					_ = level.Error(logger).Log("msg", "Failed to initialize Pulsar serializer", "err", err, "pulsar.serializer", cfg.pulsarSerializer)
					os.Exit(1)
				}
				c.WithSerializer(serializer)
				break
			}
			_ = level.Error(logger).Log("msg", "Unknown serializier confing", "pulsar.serializer", cfg.pulsarSerializer)
			os.Exit(1)
		}

		writers = append(writers, c)
	}
	_ = level.Info(logger).Log("msg", "Starting up...")
	return writers, readers
}

func serve(logger log.Logger, cfg *config, server *http.Server, writers []writer, readers []reader) error {

	mux := http.NewServeMux()
	mux.Handle(cfg.telemetryPath, promhttp.Handler())
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			_ = level.Warn(logger).Log("msg", "Error replying to health check")
		}
	})

	middleware := func(next http.HandlerFunc) http.Handler {
		return mcontext.TenantIDHandler(next)
	}

	mux.Handle(cfg.writePath, middleware(func(w http.ResponseWriter, r *http.Request) {
		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			_ = level.Error(logger).Log("msg", "Read error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			_ = level.Error(logger).Log("msg", "Decode error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			_ = level.Error(logger).Log("msg", "Unmarshal error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		samples := protoToSamples(&req)
		receivedSamples.Add(float64(len(samples)))

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
				errs[pos] = sendSamples(logger, rw, ctx, samples)
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

	mux.Handle("/read", middleware(func(w http.ResponseWriter, r *http.Request) {
		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			_ = level.Error(logger).Log("msg", "Read error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			_ = level.Error(logger).Log("msg", "Decode error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.ReadRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			_ = level.Error(logger).Log("msg", "Unmarshal error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// TODO: Support reading from more than one reader and merging the results.
		if len(readers) != 1 {
			http.Error(w, fmt.Sprintf("expected exactly one reader, found %d readers", len(readers)), http.StatusInternalServerError)
			return
		}
		reader := readers[0]

		var resp *prompb.ReadResponse
		resp, err = reader.Read(&req)
		if err != nil {
			_ = level.Warn(logger).Log("msg", "Error executing query", "query", req, "storage", reader.Name(), "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err := proto.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Set("Content-Encoding", "snappy")

		compressed = snappy.Encode(nil, data)
		if _, err := w.Write(compressed); err != nil {
			_ = level.Warn(logger).Log("msg", "Error writing response", "storage", reader.Name(), "err", err)
		}
	}))

	server.Handler = mux
	return server.ListenAndServe()
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

func sendSamples(logger log.Logger, w writer, ctx context.Context, samples model.Samples) error {
	begin := time.Now()
	err := w.Write(ctx, samples)
	duration := time.Since(begin).Seconds()
	sentSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	sentBatchDuration.WithLabelValues(w.Name()).Observe(duration)
	if err != nil {
		_ = level.Warn(logger).Log("msg", errSendingSamples, "err", err, "storage", w.Name(), "num_samples", len(samples))
		failedSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
		return err
	}
	return nil
}
