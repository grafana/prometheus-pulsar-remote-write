package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"

	"github.com/grafana/prometheus-pulsar-remote-write/pkg/pulsar"
)

type pulsarConfig struct {
	url                             string
	serializer                      string
	topic                           string
	connectTimeout                  time.Duration
	certificateAuthority            string
	clientCertificate               string
	clientKey                       string
	insecureSkipTLSVerify           bool
	insecureSkipTLSValidateHostname bool
	maxConnectionsPerBroker         int
}

var (
	pulsarSerializerJSON           = "json"
	pulsarSerializerJSONCompat     = "json-compat"
	pulsarSerializerAvroJSONCompat = "avro-json-compat"
)

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

func (cfg *pulsarConfig) addFlags(a flagger) {
	a.Flag("pulsar.url", "The URL of the remote Pulsar server to send samples to. Example: pulsar://pulsar-proxy:6650 or pulsar+ssl://pulsar-proxy:6651. None, if empty.").
		Default("").StringVar(&cfg.url)
	a.Flag("pulsar.connection-timeout", "The timeout to use when connection to the remote Pulsar server.").
		Default("30s").DurationVar(&cfg.connectTimeout)
	a.Flag("pulsar.serializer", pulsarSerializerHelp).
		Default("json").StringVar(&cfg.serializer)
	a.Flag("pulsar.topic", "The Pulsar topic to use for publishing or subscribing to metrics on.").
		Default("metrics").StringVar(&cfg.topic)
	a.Flag("pulsar.certificate-authority", "Path to the file that containing the trusted certificate authority for the connection to Pulsar.").
		Default("").StringVar(&cfg.certificateAuthority)
	a.Flag("pulsar.client-certificate", "Path to the file containing the client certificate used for the connection to Pulsar.").
		Default("").StringVar(&cfg.clientCertificate)
	a.Flag("pulsar.client-key", "Path to the file containing the client key used for the connection to Pulsar.").
		Default("").StringVar(&cfg.clientKey)
	a.Flag("pulsar.insecure-skip-tls-verify", "Configure whether the Pulsar client accepts untrusted TLS certificate from broker.").
		Default("false").BoolVar(&cfg.insecureSkipTLSVerify)
	a.Flag("pulsar.insecure-skip-tls-validate-hostname", "Configure whether the Pulsar client skips to verify the validity of the host name from broker.").
		Default("false").BoolVar(&cfg.insecureSkipTLSValidateHostname)
	a.Flag("pulsar.max-connections-per-broker", "Max number of connections to a single broker that will kept in the pool.").
		Default("1").IntVar(&cfg.maxConnectionsPerBroker)
}

func (cfg *pulsarConfig) clientOptions() (*pulsar.ClientOptions, error) {
	// set authentication method if necessary
	var auth pulsar.Authentication
	if cfg.clientKey != "" || cfg.clientCertificate != "" {
		if cfg.clientKey == "" || cfg.clientCertificate == "" {
			return nil, fmt.Errorf("both pulsar.client-key and pulsar.client-certificate need to be specified")
		}
		auth = pulsar.NewAuthenticationTLS(cfg.clientCertificate, cfg.clientKey)
	}
	return &pulsar.ClientOptions{
		URL:               cfg.url,
		ConnectionTimeout: cfg.connectTimeout,

		TLSTrustCertsFilePath:      cfg.certificateAuthority,
		Authentication:             auth,
		TLSAllowInsecureConnection: cfg.insecureSkipTLSVerify,
		TLSValidateHostname:        !cfg.insecureSkipTLSValidateHostname,
		MaxConnectionsPerBroker:    cfg.maxConnectionsPerBroker,
	}, nil
}

type pulsarClientOpts func(cfg *pulsar.Config)

func pulsarClientWithReplicaLabels(rl []string) pulsarClientOpts {
	return func(cfg *pulsar.Config) {
		cfg.ReplicaLabels = rl
	}
}

func pulsarClientWithSubscription(sub string) pulsarClientOpts {
	return func(cfg *pulsar.Config) {
		cfg.Subscription = sub
	}
}

func pulsarClientWithClientOpts(opts *pulsar.ClientOptions) pulsarClientOpts {
	return func(cfg *pulsar.Config) {
		cfg.ClientOptions = *opts
	}
}

func (cfg *pulsarConfig) client(logger log.Logger, opts ...pulsarClientOpts) (*pulsar.Client, error) {
	config := pulsar.Config{
		Topic:  cfg.topic,
		Logger: log.With(logger, "storage", "Pulsar"),
	}

	// execute options, if there are any
	for _, opt := range opts {
		opt(&config)
	}

	if config.ClientOptions == (pulsar.ClientOptions{}) {
		clientOptions, err := cfg.clientOptions()
		if err != nil {
			return nil, err
		}
		config.ClientOptions = *clientOptions
	}

	client, err := pulsar.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Pulsar: %w", err)
	}

	// check for the serializer settings
	switch cfg.serializer {
	case pulsarSerializerJSON:
		client.WithSerializer(pulsar.NewJSONSerializer())
	case pulsarSerializerJSONCompat:
		client.WithSerializer(pulsar.NewJSONCompatSerializer())
	case pulsarSerializerAvroJSONCompat:
		serializer, err := pulsar.NewAvroJSONSerializer(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Pulsar serializer '%s': %w", cfg.serializer, err)
		}
		client.WithSerializer(serializer)
	default:
		prefix := fmt.Sprintf("%s=", pulsarSerializerAvroJSONCompat)
		if strings.HasPrefix(cfg.serializer, prefix) {
			filePath := cfg.serializer[len(prefix):]
			file, err := os.Open(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Pulsar avro serializer schema in path '%s': %w", filePath, err)
			}
			defer file.Close()

			serializer, err := pulsar.NewAvroJSONSerializer(file)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Pulsar serializer '%s': %w", cfg.serializer, err)
			}
			client.WithSerializer(serializer)
			break
		}
		return nil, fmt.Errorf("unknown Pulsar serializer config '%s'", cfg.serializer)
	}

	return client, nil
}
